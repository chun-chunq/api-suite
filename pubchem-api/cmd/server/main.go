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
	"pubchem-api/internal/api/handler"
	"pubchem-api/internal/client"
)

var startTime = time.Now()

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8110")
	cl := client.New()

	var totalReqs, totalErrs atomic.Int64

	app := fiber.New(fiber.Config{
		AppName:               "pubchem-api",
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          30 * time.Second,
		DisableStartupMessage: true,
	})

	app.Use(recover.New())

	// PubChem rate limit: 5 req/s, 400/min — we limit to 60/min to be safe
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

	// Search by name → list of CIDs
	app.Get("/v1/chem/search", func(c *fiber.Ctx) error {
		return handler.Search(c, cl)
	})

	// Get compound by CID
	app.Get("/v1/chem/cid/:cid", func(c *fiber.Ctx) error {
		return handler.GetByCID(c, cl)
	})

	// Get synonyms for a CID
	app.Get("/v1/chem/cid/:cid/synonyms", func(c *fiber.Ctx) error {
		return handler.GetSynonyms(c, cl)
	})

	// Get description for a CID
	app.Get("/v1/chem/cid/:cid/description", func(c *fiber.Ctx) error {
		return handler.GetDescription(c, cl)
	})

	// Get compound by name (best match, full details)
	app.Get("/v1/chem/name/:name", func(c *fiber.Ctx) error {
		return handler.GetByName(c, cl)
	})

	// MCP endpoint
	app.Post("/mcp", func(c *fiber.Ctx) error {
		return handler.MCP(c, cl)
	})

	// Health
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":      "ok",
			"service":     "pubchem-api",
			"port":        port,
			"uptime":      time.Since(startTime).String(),
			"total_reqs":  totalReqs.Load(),
			"total_errs":  totalErrs.Load(),
			"data_source": "PubChem PUG REST (pubchem.ncbi.nlm.nih.gov)",
			"rate_limit":  "60 req/min",
		})
	})

	log.Info().Str("port", port).Msg("pubchem-api starting")
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
