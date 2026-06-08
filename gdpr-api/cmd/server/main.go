package main

import (
	"os"
	"sync/atomic"
	"time"

	"gdpr-api/internal/api/handler"
	"gdpr-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	startTime     = time.Now()
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.With().Str("service", "gdpr-api").Logger()

	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "changeme"
	}

	c := client.New()
	gdprH := handler.NewGDPRHandler(c, logger)
	mcpH := handler.NewMCPHandler(c, logger)

	app := fiber.New(fiber.Config{
		ReadTimeout:  35 * time.Second,
		WriteTimeout: 35 * time.Second,
	})
	app.Use(recover.New())

	app.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		totalRequests.Add(1)
		if c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		count, cachedAt := client.New().CacheInfo()
		_ = count
		_ = cachedAt
		return c.JSON(fiber.Map{"status": "ok", "service": "gdpr-api", "port": 8097})
	})

	rl := limiter.New(limiter.Config{
		Max:        120,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			if k := c.Get("X-API-Key"); k != "" { return k }
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "rate limit exceeded — 120 req/min"})
		},
	})

	v1 := app.Group("/v1", rl)
	v1.Get("/gdpr/fines", gdprH.Search)
	v1.Get("/gdpr/top", gdprH.TopFines)
	v1.Get("/gdpr/cache-status", gdprH.CacheStatus)

	app.Post("/mcp", mcpH.Handle)

	adm := app.Group("/admin", adminAuth(adminSecret))
	adm.Get("/analytics", func(c *fiber.Ctx) error {
		reqs := totalRequests.Load()
		errs := totalErrors.Load()
		rate := 0.0
		if reqs > 0 { rate = float64(errs) / float64(reqs) * 100 }
		return c.JSON(fiber.Map{
			"totalRequests": reqs, "totalErrors": errs,
			"errorRatePct":  rate,
			"uptimeSeconds": int(time.Since(startTime).Seconds()),
			"service":       "gdpr-api",
		})
	})

	logger.Info().Msg("gdpr-api listening on :8097")
	if err := app.Listen(":8097"); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}

func adminAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("X-Admin-Secret") != secret {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
}
