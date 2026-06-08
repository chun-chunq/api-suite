package main

import (
	"os"
	"sync/atomic"
	"time"

	"euipo-api/internal/api/handler"
	"euipo-api/internal/client"

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
	logger := log.With().Str("service", "euipo-api").Logger()

	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "changeme"
	}

	tmviewClient := client.New()
	trademarkH := handler.NewTrademarkHandler(tmviewClient, logger)
	mcpH := handler.NewMCPHandler(tmviewClient, logger, adminSecret)

	app := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	app.Use(recover.New())

	// Counter middleware
	app.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		totalRequests.Add(1)
		if c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "euipo-api", "port": 8093})
	})

	// Rate limiter: 120 req/min per IP
	rl := limiter.New(limiter.Config{
		Max:        120,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			if key := c.Get("X-RapidAPI-Proxy-Secret"); key != "" {
				return "rapid:" + c.Get("X-RapidAPI-User")
			}
			return c.Get("X-API-Key")
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "rate limit exceeded — 120 requests per minute"})
		},
	})

	v1 := app.Group("/v1", rl)

	// Trademark endpoints
	v1.Get("/trademark/search", trademarkH.Search)
	v1.Get("/trademark/:office/:appNum", trademarkH.GetByID)

	// MCP endpoint (no rate limit — used by AI agents)
	app.Post("/mcp", mcpH.Handle)

	// Admin endpoints
	adm := app.Group("/admin", adminAuth(adminSecret))
	adm.Get("/analytics", func(c *fiber.Ctx) error {
		reqs := totalRequests.Load()
		errs := totalErrors.Load()
		errRate := 0.0
		if reqs > 0 {
			errRate = float64(errs) / float64(reqs) * 100
		}
		return c.JSON(fiber.Map{
			"totalRequests": reqs,
			"totalErrors":   errs,
			"errorRatePct":  errRate,
			"uptimeSeconds": int(time.Since(startTime).Seconds()),
			"service":       "euipo-api",
		})
	})

	logger.Info().Msg("euipo-api listening on :8093")
	if err := app.Listen(":8093"); err != nil {
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
