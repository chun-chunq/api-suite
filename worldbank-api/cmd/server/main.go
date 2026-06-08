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
	"worldbank-api/internal/api/handler"
	"worldbank-api/internal/client"
)

var (
	reqCount atomic.Int64
	errCount atomic.Int64
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8120")

	cl := client.New()

	app := fiber.New(fiber.Config{
		AppName:               "worldbank-api",
		DisableStartupMessage: true,
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
	})

	app.Use(recover.New())
	app.Use(limiter.New(limiter.Config{
		Max:        60,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded — max 60 requests/min per IP",
			})
		},
	}))

	app.Use(func(c *fiber.Ctx) error {
		reqCount.Add(1)
		err := c.Next()
		if c.Response().StatusCode() >= 500 {
			errCount.Add(1)
		}
		return err
	})

	// ── Routes ────────────────────────────────────────────────────────────────
	v1 := app.Group("/v1/worldbank")
	v1.Get("/country/:code", func(c *fiber.Ctx) error { return handler.GetCountry(c, cl) })
	v1.Get("/indicator", func(c *fiber.Ctx) error { return handler.GetIndicator(c, cl) })
	v1.Get("/indicators", func(c *fiber.Ctx) error { return handler.ListIndicators(c, cl) })

	// MCP endpoint
	app.Post("/mcp", func(c *fiber.Ctx) error { return handler.MCP(c, cl) })

	// Health + stats
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "worldbank-api",
			"port":    port,
		})
	})
	app.Get("/stats", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"requests": reqCount.Load(),
			"errors":   errCount.Load(),
		})
	})

	log.Info().Str("port", port).Msg("worldbank-api starting")
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
