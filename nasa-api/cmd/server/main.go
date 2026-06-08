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
	"nasa-api/internal/api/handler"
	"nasa-api/internal/client"
)

var startTime = time.Now()

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8111")
	apiKey := envOr("NASA_API_KEY", "") // uses DEMO_KEY if empty
	cl := client.New(apiKey)

	var totalReqs, totalErrs atomic.Int64

	app := fiber.New(fiber.Config{
		AppName:               "nasa-api",
		ReadTimeout:           25 * time.Second,
		WriteTimeout:          25 * time.Second,
		DisableStartupMessage: true,
	})

	app.Use(recover.New())

	// NASA with DEMO_KEY: 30 req/hour. With registered key: 1000 req/hour.
	// We limit conservatively at 30/min.
	app.Use(limiter.New(limiter.Config{
		Max:        30,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded: 30 requests/minute",
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

	app.Get("/v1/nasa/apod", func(c *fiber.Ctx) error {
		return handler.APOD(c, cl)
	})
	app.Get("/v1/nasa/apod/range", func(c *fiber.Ctx) error {
		return handler.APODRange(c, cl)
	})
	app.Get("/v1/nasa/mars/:rover/photos", func(c *fiber.Ctx) error {
		return handler.MarsPhotos(c, cl)
	})
	app.Get("/v1/nasa/neo", func(c *fiber.Ctx) error {
		return handler.NEOFeed(c, cl)
	})

	app.Post("/mcp", func(c *fiber.Ctx) error {
		return handler.MCP(c, cl)
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		keyStatus := "DEMO_KEY (limited)"
		if apiKey != "" && apiKey != "DEMO_KEY" {
			keyStatus = "registered key"
		}
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "nasa-api",
			"port":        port,
			"uptime":      time.Since(startTime).String(),
			"total_reqs":  totalReqs.Load(),
			"total_errs":  totalErrs.Load(),
			"data_source": "NASA Open APIs (api.nasa.gov)",
			"api_key":     keyStatus,
			"rate_limit":  "30 req/min",
		})
	})

	log.Info().Str("port", port).Msg("nasa-api starting")
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
