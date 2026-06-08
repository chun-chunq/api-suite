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
	"openfda-api/internal/api/handler"
	"openfda-api/internal/client"
)

var (
	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	startTime     = time.Now()
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8103")
	adminSecret := envOr("ADMIN_SECRET", "change-me")
	apiKey := os.Getenv("OPENFDA_API_KEY") // optional

	c := client.New(apiKey)
	dh := handler.NewDrugHandler(c, log.Logger)
	mh := handler.NewMCPHandler(c, log.Logger)

	app := fiber.New(fiber.Config{
		AppName:               "openfda-api",
		ReadTimeout:           25 * time.Second,
		WriteTimeout:          25 * time.Second,
		DisableStartupMessage: true,
	})
	app.Use(recover.New())
	app.Use(func(c *fiber.Ctx) error {
		totalRequests.Add(1)
		err := c.Next()
		if err != nil || c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})
	app.Use(limiter.New(limiter.Config{
		Max:        60,
		Expiration: 1 * time.Minute,
	}))

	// ── Drug endpoints
	app.Get("/v1/drug/labels", dh.SearchLabels)
	app.Get("/v1/drug/events", dh.SearchEvents)
	app.Get("/v1/drug/recalls", dh.SearchRecalls)

	// ── MCP
	app.Post("/mcp", mh.Handle)

	// ── Admin
	adminAuth := func(c *fiber.Ctx) error {
		if c.Get("X-Admin-Secret") != adminSecret {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
	app.Get("/admin/stats", adminAuth, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service":       "openfda-api",
			"totalRequests": totalRequests.Load(),
			"totalErrors":   totalErrors.Load(),
			"uptime":        time.Since(startTime).String(),
		})
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "openfda-api", "port": port})
	})

	log.Info().Str("port", port).Msg("openfda-api starting")
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
