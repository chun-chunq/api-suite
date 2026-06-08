package main

import (
	"os"
	"sync/atomic"
	"time"

	"food-api/internal/api/handler"
	"food-api/internal/client"

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
	logger := log.With().Str("service", "food-api").Logger()

	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "changeme"
	}

	c := client.New()
	foodH := handler.NewFoodHandler(c, logger)
	mcpH := handler.NewMCPHandler(c, logger)

	app := fiber.New(fiber.Config{
		ReadTimeout:  25 * time.Second,
		WriteTimeout: 25 * time.Second,
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
		return c.JSON(fiber.Map{"status": "ok", "service": "food-api", "port": 8099})
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
	v1.Get("/food/product/:barcode", foodH.GetByBarcode)
	v1.Get("/food/search", foodH.Search)

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
			"service":       "food-api",
		})
	})

	logger.Info().Msg("food-api listening on :8099")
	if err := app.Listen(":8099"); err != nil {
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
