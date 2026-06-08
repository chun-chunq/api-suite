// Command worker runs a lightweight ZVG scrape worker.
// No auth — only expose on private network (Tailscale, VPN, or LAN).
//
// Run: WORKER_PORT=9091 go run ./cmd/worker
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

	"github.com/zvg-api/internal/scraper"
)

func main() {
	port      := getenv("WORKER_PORT", "9091")
	chromeBin := getenv("CHROME_BIN", "")

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("role", "zvg-worker").Logger()
	log.Logger = logger

	logger.Info().Str("port", port).Msg("starting ZVG scrape worker")

	app := fiber.New(fiber.Config{
		AppName:               "zvg-worker",
		DisableStartupMessage: true,
		ReadTimeout:           100 * time.Second,
		WriteTimeout:          100 * time.Second,
	})
	app.Use(recover.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "role": "zvg-scrape-worker"})
	})

	app.Post("/scrape/zvg", func(c *fiber.Ctx) error {
		var q scraper.SearchQuery
		if err := c.BodyParser(&q); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		sc, err := scraper.New(scraper.Options{Logger: logger, BrowserBin: chromeBin})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		defer sc.Close()

		ctx, cancel := context.WithTimeout(c.Context(), 80*time.Second)
		defer cancel()

		res, err := sc.Search(ctx, q)
		if err != nil {
			// 503 → pool records failure and retries on next worker
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(res)
	})

	app.Post("/scrape/zvg/courts", func(c *fiber.Ctx) error {
		var body struct{ State string `json:"state"` }
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		sc, err := scraper.New(scraper.Options{Logger: logger, BrowserBin: chromeBin})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		defer sc.Close()

		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()

		courts, err := sc.GetCourts(ctx, body.State)
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(courts)
	})

	go func() {
		if err := app.Listen(":" + port); err != nil {
			logger.Fatal().Err(err).Msg("worker stopped")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
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
