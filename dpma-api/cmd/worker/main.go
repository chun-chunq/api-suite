// Standalone scrape worker for DPMA trademark lookups.
// Run on a separate server/IP to rotate away from blocks.
// Port: 9092 (set via WORKER_PORT env var)
package main

import (
	"context"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/dpma-api/internal/scraper"
)

func main() {
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
		With().Timestamp().Str("role", "worker").Logger()

	port := os.Getenv("WORKER_PORT")
	if port == "" {
		port = "9092"
	}
	chromeBin := os.Getenv("CHROME_BIN")

	app := fiber.New(fiber.Config{
		AppName:               "dpma-worker",
		DisableStartupMessage: true,
		ReadTimeout:           90 * time.Second,
		WriteTimeout:          90 * time.Second,
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "worker": "dpma"})
	})

	app.Post("/scrape/trademark", func(c *fiber.Ctx) error {
		var q scraper.SearchQuery
		if err := c.BodyParser(&q); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid query"})
		}
		ctx, cancel := context.WithTimeout(c.Context(), 70*time.Second)
		defer cancel()

		sc, err := scraper.New(scraper.Options{Logger: log, BrowserBin: chromeBin})
		if err != nil {
			return c.Status(503).JSON(fiber.Map{"error": err.Error()})
		}
		defer sc.Close()

		result, err := sc.Search(ctx, q)
		if err != nil {
			return c.Status(503).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(result)
	})

	log.Info().Str("port", port).Msg("dpma scrape worker started")
	if err := app.Listen(":" + port); err != nil {
		log.Fatal().Err(err).Msg("worker server error")
	}
}
