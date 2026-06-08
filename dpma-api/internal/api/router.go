package api

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"

	"github.com/dpma-api/config"
	"github.com/dpma-api/internal/analytics"
	"github.com/dpma-api/internal/api/handler"
	"github.com/dpma-api/internal/api/middleware"
	"github.com/dpma-api/internal/cache"
	"github.com/dpma-api/internal/jobqueue"
	"github.com/dpma-api/internal/pool"
	"github.com/dpma-api/internal/scrapequeue"
)

func NewRouter(cfg *config.Config, c *cache.Cache, log zerolog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "dpma-trademark-api",
		DisableStartupMessage: true,
		ReadTimeout:           80 * time.Second,
		WriteTimeout:          80 * time.Second,
		// Reduce idle memory: disable preforking, keep default stack
		Prefork: false,
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(requestLogger(log))
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.LegalHeaders())
	app.Use(middleware.SanitizeInputs()) // injection protection on ALL query params

	// Bandwidth tracking (Hetzner soft limit)
	bw := middleware.NewBandwidthTracker(cfg.BandwidthLimitGB, log)
	app.Use(bw.Middleware())

	// Dynamic worker pool (hot-add/remove via /admin/workers)
	workerPool := pool.New(cfg.WorkerURLs, log)
	if len(cfg.WorkerURLs) > 0 {
		log.Info().Strs("workers", cfg.WorkerURLs).Msg("scrape worker pool configured")
	}

	// PC-Worker bridge (home-PC long-poll)
	jq := jobqueue.New()
	if cfg.WorkerSecret != "" {
		bridge := handler.NewWorkerBridgeHandler(jq, cfg.WorkerSecret)
		internal := app.Group("/internal/worker")
		internal.Get("/poll", bridge.Poll)
		internal.Post("/result/:id", bridge.Result)
		log.Info().Msg("PC-Worker bridge enabled")
	}

	// Bounded scrape queue (replaces raw semaphore; includes depth limiting)
	// MAX_BROWSERS = concurrent Chrome slots; MAX_QUEUE_DEPTH = max requests waiting
	q := scrapequeue.New(cfg.MaxBrowsers, cfg.MaxQueueDepth)
	log.Info().
		Int("browserSlots", cfg.MaxBrowsers).
		Int("maxQueueDepth", cfg.MaxQueueDepth).
		Msg("scrape queue configured")

	// Analytics (atomic counters + ring buffer, zero-cost on hot path)
	an := analytics.New(log)

	// Admin endpoints (dynamic worker management + stats + analytics)
	if cfg.AdminSecret != "" {
		adminAuth := adminKeyMiddleware(cfg.AdminSecret)
		adm := handler.NewAdminHandler(workerPool, bw, an, q)
		admin := app.Group("/admin", adminAuth)
		admin.All("/workers", adm.Workers)
		admin.Get("/stats", adm.Stats)
		admin.Get("/analytics", adm.Analytics)
		log.Info().Msg("Admin API enabled on /admin/{workers,stats,analytics}")
	}

	// Health — no auth
	health := handler.NewHealthHandler(c, workerPool)
	app.Get("/health", health.Health)

	// Auth + rate limiting
	app.Use(middleware.APIKey(cfg.APIKeys, "/health", "/internal/", "/admin/", "/mcp"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))

	// Trademark endpoints
	tm := handler.NewTrademarkHandler(c, log, cfg.CacheTTL, cfg.ChromeBin, workerPool, jq, q, an)
	v1 := app.Group("/v1/trademark")
	v1.Get("/search", tm.Search)
	v1.Get("/:number", tm.Detail)

	// MCP endpoint (no API key — AI tools use this directly)
	mcp := handler.NewMCPHandler(tm, log)
	app.Post("/mcp", mcp.Handle)

	// Apify Actor endpoint
	apify := handler.NewApifyHandler(tm, log)
	app.Post("/apify/run", apify.Run)

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
			Str("ip", c.IP()).
			Msg("req")
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
