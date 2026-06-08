// Command worker runs a lightweight HTTP scrape worker.
// It has NO auth and should only be reachable on a private network (Tailscale, VPN, or LAN).
// The main API dispatches requests to it when it needs a different exit IP.
//
// Run: WORKER_PORT=9090 go run ./cmd/worker
// Or with Docker: docker run -e WORKER_PORT=9090 -e CHROME_BIN=/usr/bin/chromium insolvency-worker
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/insolvency-api/internal/scraper"
)

func main() {
	port := getenv("WORKER_PORT", "9090")
	chromeBin := getenv("CHROME_BIN", "")

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("role", "worker").Logger()
	log.Logger = logger

	logger.Info().Str("port", port).Msg("starting insolvency scrape worker")

	app := fiber.New(fiber.Config{
		AppName:               "insolvency-worker",
		DisableStartupMessage: true,
		ReadTimeout:           100 * time.Second,
		WriteTimeout:          100 * time.Second,
	})
	app.Use(recover.New())

	// Health check — used by the pool to verify the worker is alive.
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "role": "insolvency-scrape-worker"})
	})

	// Scrape endpoint — accepts a SearchQuery JSON body, returns a SearchResult JSON.
	app.Post("/scrape/insolvency", func(c *fiber.Ctx) error {
		var q scraper.SearchQuery
		if err := c.BodyParser(&q); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body: " + err.Error()})
		}

		sc, err := scraper.New(scraper.Options{
			Logger:     logger,
			BrowserBin: chromeBin,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "browser init: " + err.Error()})
		}
		defer sc.Close()

		ctx, cancel := context.WithTimeout(c.Context(), 80*time.Second)
		defer cancel()

		res, err := sc.Search(ctx, q)
		if err != nil {
			// Return 503 so the pool records a failure and tries the next worker.
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(res)
	})

	go func() {
		if err := app.Listen(":" + port); err != nil {
			logger.Fatal().Err(err).Msg("worker stopped")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info().Msg("worker shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	app.ShutdownWithContext(ctx)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
