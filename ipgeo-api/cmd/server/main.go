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
	"ipgeo-api/internal/api/handler"
	"ipgeo-api/internal/client"
)

var (
	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	startTime     = time.Now()
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8107")
	adminSecret := envOr("ADMIN_SECRET", "change-me")
	apiKey := os.Getenv("IPAPI_KEY") // optional paid key

	c := client.New(apiKey)
	gh := handler.NewGeoHandler(c, log.Logger)
	mh := handler.NewMCPHandler(c, log.Logger)

	app := fiber.New(fiber.Config{
		AppName:               "ipgeo-api",
		ReadTimeout:           15 * time.Second,
		WriteTimeout:          15 * time.Second,
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
	// Conservative rate limit — ip-api.com free tier is 45 req/min
	app.Use(limiter.New(limiter.Config{
		Max:        40,
		Expiration: 1 * time.Minute,
	}))

	app.Get("/v1/ipgeo/lookup", gh.Lookup)       // caller's own IP
	app.Get("/v1/ipgeo/lookup/:ip", gh.Lookup)   // specific IP
	app.Post("/v1/ipgeo/batch", gh.Batch)         // up to 100 IPs

	app.Post("/mcp", mh.Handle)

	adminAuth := func(c *fiber.Ctx) error {
		if c.Get("X-Admin-Secret") != adminSecret {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
	app.Get("/admin/stats", adminAuth, func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service":       "ipgeo-api",
			"totalRequests": totalRequests.Load(),
			"totalErrors":   totalErrors.Load(),
			"uptime":        time.Since(startTime).String(),
		})
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "ipgeo-api", "port": port})
	})

	log.Info().Str("port", port).Msg("ipgeo-api starting")
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
