package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"

	"strings"

	"github.com/zvg-api/config"
	"github.com/zvg-api/internal/analytics"
	"github.com/zvg-api/internal/api/handler"
	"github.com/zvg-api/internal/api/middleware"
	"github.com/zvg-api/internal/cache"
	"github.com/zvg-api/internal/jobqueue"
	"github.com/zvg-api/internal/pool"
	"github.com/zvg-api/internal/scrapequeue"
)

func NewRouter(cfg *config.Config, c *cache.Cache, log zerolog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "zvg-api",
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

	health := handler.NewHealthHandler(c)
	app.Get("/health", health.Health)

	app.Use(middleware.APIKey(cfg.APIKeys, "/health", "/internal/", "/admin/"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))

	zvg := handler.NewZVGHandler(c, log, cfg.CacheTTL, cfg.ChromeBin, workerPool, jq, q, an)
	v1 := app.Group("/v1/zvg")
	v1.Get("/search", zvg.Search)
	v1.Get("/courts", zvg.Courts)

	return app
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
