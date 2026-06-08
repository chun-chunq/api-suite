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
	"exchangerate-api/internal/api/handler"
	"exchangerate-api/internal/client"
)

var startTime = time.Now()

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8114")
	cl := client.New()

	var totalReqs, totalErrs atomic.Int64

	app := fiber.New(fiber.Config{
		AppName:               "exchangerate-api",
		ReadTimeout:           20 * time.Second,
		WriteTimeout:          20 * time.Second,
		DisableStartupMessage: true,
	})

	app.Use(recover.New())

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

	app.Use(func(c *fiber.Ctx) error {
		totalReqs.Add(1)
		err := c.Next()
		if c.Response().StatusCode() >= 500 {
			totalErrs.Add(1)
		}
		return err
	})

	// ── Routes ────────────────────────────────────────────────────────────────

	app.Get("/v1/fx/latest", func(c *fiber.Ctx) error {
		return handler.Latest(c, cl)
	})
	app.Get("/v1/fx/historical/:date", func(c *fiber.Ctx) error {
		return handler.Historical(c, cl)
	})
	app.Get("/v1/fx/series", func(c *fiber.Ctx) error {
		return handler.TimeSeries(c, cl)
	})
	app.Get("/v1/fx/convert", func(c *fiber.Ctx) error {
		return handler.Convert(c, cl)
	})
	app.Get("/v1/fx/currencies", func(c *fiber.Ctx) error {
		return handler.Currencies(c, cl)
	})

	app.Post("/mcp", func(c *fiber.Ctx) error {
		return handler.MCP(c, cl)
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "exchangerate-api",
			"port":        port,
			"uptime":      time.Since(startTime).String(),
			"total_reqs":  totalReqs.Load(),
			"total_errs":  totalErrs.Load(),
			"data_source": "Frankfurter (ECB data, frankfurter.app)",
			"history":     "back to 1999-01-04",
			"rate_limit":  "60 req/min",
		})
	})

	log.Info().Str("port", port).Msg("exchangerate-api starting")
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
