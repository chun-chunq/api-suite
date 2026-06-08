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
	"gbif-api/internal/api/handler"
	"gbif-api/internal/client"
)

var startTime = time.Now()

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8115")
	cl := client.New()

	var totalReqs, totalErrs atomic.Int64

	app := fiber.New(fiber.Config{
		AppName:               "gbif-api",
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

	app.Get("/v1/bio/species", func(c *fiber.Ctx) error {
		return handler.SearchSpecies(c, cl)
	})
	app.Get("/v1/bio/species/:key/names", func(c *fiber.Ctx) error {
		return handler.GetVernacularNames(c, cl)
	})
	app.Get("/v1/bio/species/:key", func(c *fiber.Ctx) error {
		return handler.GetSpecies(c, cl)
	})
	app.Get("/v1/bio/occurrences", func(c *fiber.Ctx) error {
		return handler.SearchOccurrences(c, cl)
	})

	app.Post("/mcp", func(c *fiber.Ctx) error {
		return handler.MCP(c, cl)
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "gbif-api",
			"port":        port,
			"uptime":      time.Since(startTime).String(),
			"total_reqs":  totalReqs.Load(),
			"total_errs":  totalErrs.Load(),
			"data_source": "GBIF (Global Biodiversity Information Facility, api.gbif.org)",
			"rate_limit":  "60 req/min",
		})
	})

	log.Info().Str("port", port).Msg("gbif-api starting")
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
