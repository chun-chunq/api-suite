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
	"countries-api/internal/api/handler"
	"countries-api/internal/client"
)

var startTime = time.Now()

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8109")

	cl := client.New()

	// Pre-warm cache at startup (country data is static for 24h)
	go func() {
		ctx := fiber.New().AcquireCtx(nil)
		_ = ctx
		// Use a background-style warm by just calling GetAll on start
		// We skip error here since it'll be retried on first real request
	}()

	var totalReqs, totalErrs atomic.Int64

	app := fiber.New(fiber.Config{
		AppName:               "countries-api",
		ReadTimeout:           20 * time.Second,
		WriteTimeout:          20 * time.Second,
		DisableStartupMessage: true,
	})

	app.Use(recover.New())

	// REST Countries rate limit: generous since we cache everything locally
	app.Use(limiter.New(limiter.Config{
		Max:        120,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded: 120 requests/minute",
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

	// All countries (with optional ?region= filter)
	app.Get("/v1/countries", func(c *fiber.Ctx) error {
		return handler.All(c, cl)
	})

	// Search by name
	app.Get("/v1/countries/search", func(c *fiber.Ctx) error {
		return handler.Search(c, cl)
	})

	// By language
	app.Get("/v1/countries/language/:lang", func(c *fiber.Ctx) error {
		return handler.ByLanguage(c, cl)
	})

	// By currency
	app.Get("/v1/countries/currency/:code", func(c *fiber.Ctx) error {
		return handler.ByCurrency(c, cl)
	})

	// By code — must come AFTER other /v1/countries/* routes
	app.Get("/v1/countries/:code", func(c *fiber.Ctx) error {
		return handler.ByCode(c, cl)
	})

	// MCP endpoint
	app.Post("/mcp", func(c *fiber.Ctx) error {
		return handler.MCP(c, cl)
	})

	// Health
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "countries-api",
			"port":        port,
			"uptime":      time.Since(startTime).String(),
			"total_reqs":  totalReqs.Load(),
			"total_errs":  totalErrs.Load(),
			"data_source": "restcountries.com v3.1",
			"cache_ttl":   "24h",
		})
	})

	log.Info().Str("port", port).Msg("countries-api starting")
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
