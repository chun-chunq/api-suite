package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"

	"strings"

	"github.com/insolvency-api/config"
	"github.com/insolvency-api/internal/analytics"
	"github.com/insolvency-api/internal/api/handler"
	"github.com/insolvency-api/internal/api/middleware"
	"github.com/insolvency-api/internal/cache"
	"github.com/insolvency-api/internal/jobqueue"
	"github.com/insolvency-api/internal/pool"
	"github.com/insolvency-api/internal/scrapequeue"
)

// NewRouter wires the Fiber app with all routes and middleware.
func NewRouter(cfg *config.Config, c *cache.Cache, log zerolog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "insolvency-api",
		DisableStartupMessage: true,
		ReadTimeout:           70 * time.Second,
		WriteTimeout:          70 * time.Second,
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(requestLogger(log))
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.LegalHeaders())
	app.Use(middleware.SanitizeInputs())

	workerPool := pool.New(cfg.WorkerURLs, log)
	if len(cfg.WorkerURLs) > 0 {
		log.Info().Strs("workers", cfg.WorkerURLs).Msg("scrape worker pool configured")
	}

	jq := jobqueue.New()
	if cfg.WorkerSecret != "" {
		bridge := handler.NewWorkerBridgeHandler(jq, cfg.WorkerSecret)
		internal := app.Group("/internal/worker")
		internal.Get("/poll", bridge.Poll)
		internal.Post("/result/:id", bridge.Result)
		log.Info().Msg("PC-Worker bridge enabled")
	}

	q := scrapequeue.New(cfg.MaxBrowsers, cfg.MaxQueueDepth)
	an := analytics.New(log)
	log.Info().Int("slots", cfg.MaxBrowsers).Int("maxDepth", cfg.MaxQueueDepth).Msg("scrape queue configured")

	if cfg.AdminSecret != "" {
		adm := handler.NewAdminHandler(workerPool, nil, an, q)
		admin := app.Group("/admin", adminKeyMiddleware(cfg.AdminSecret))
		admin.All("/workers", adm.Workers)
		admin.Get("/stats", adm.Stats)
		admin.Get("/analytics", adm.Analytics)
		log.Info().Msg("Admin API enabled on /admin/{workers,stats,analytics}")
	}

	health := handler.NewHealthHandler(c, workerPool)
	app.Get("/health", health.Health)

	app.Use(middleware.APIKey(cfg.APIKeys, "/health", "/internal/", "/admin/"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))

	ins := handler.NewInsolvencyHandler(c, log, cfg.SearchCacheTTL, cfg.RecordCacheTTL, workerPool, jq, q, an, cfg.ChromeBin)

	v1 := app.Group("/v1/insolvency")
	v1.Get("/search", ins.Search)
	v1.Get("/company/:hrb", ins.Company)
	v1.Get("/monitor", ins.Monitor)

	return app
}

func requestLogger(log zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		log.Info().
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).
			Dur("latency", time.Since(start)).
			Msg("request")
		return err
	}
}

func adminKeyMiddleware(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("X-Admin-Secret")
		if key == "" {
			key = strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		}
		if key != secret {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
}
