package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"vat-api/internal/api/handler"
	"vat-api/internal/client"
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8108")

	cl := client.New()

	var totalReqs, totalErrs atomic.Int64

	app := fiber.New(fiber.Config{
		AppName:               "vat-api",
		ReadTimeout:           20 * time.Second,
		WriteTimeout:          20 * time.Second,
		DisableStartupMessage: true,
	})

	app.Use(recover.New())

	// Rate limiter: VIES is a public EU API — be respectful, max 60 req/min
	app.Use(limiter.New(limiter.Config{
		Max:        60,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded: 60 requests/minute",
			})
		},
	}))

	// Request counter middleware
	app.Use(func(c *fiber.Ctx) error {
		totalReqs.Add(1)
		err := c.Next()
		if c.Response().StatusCode() >= 500 {
			totalErrs.Add(1)
		}
		return err
	})

	// ── Routes ────────────────────────────────────────────────────────────────

	// Validate a single VAT number
	app.Get("/v1/vat/validate", func(c *fiber.Ctx) error {
		return handler.Validate(c, cl)
	})

	// Batch validate up to 10 VAT numbers
	app.Post("/v1/vat/batch", func(c *fiber.Ctx) error {
		return handler.BatchValidate(c, cl)
	})

	// List supported EU country codes
	app.Get("/v1/vat/countries", handler.Countries)

	// MCP endpoint
	app.Post("/mcp", func(c *fiber.Ctx) error {
		return handler.MCP(c, cl)
	})

	// Health
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "vat-api",
			"port":        port,
			"uptime":      time.Since(startTime).String(),
			"total_reqs":  totalReqs.Load(),
			"total_errs":  totalErrs.Load(),
			"data_source": "EU VIES (ec.europa.eu/taxation_customs/vies)",
			"rate_limit":  "60 req/min",
		})
	})

	log.Info().Str("port", port).Msg("vat-api starting")
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

var startTime = time.Now()

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
