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
	"crypto-api/internal/api/handler"
	"crypto-api/internal/client"
)

var (
	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	startTime     = time.Now()
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8105")
	adminSecret := envOr("ADMIN_SECRET", "change-me")
	apiKey := os.Getenv("COINGECKO_API_KEY") // optional demo key

	c := client.New(apiKey)
	ch := handler.NewCryptoHandler(c, log.Logger)
	mh := handler.NewMCPHandler(c, log.Logger)

	app := fiber.New(fiber.Config{
		AppName:               "crypto-api",
		ReadTimeout:           20 * time.Second,
		WriteTimeout:          20 * time.Second,
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
	// Strict rate limit — CoinGecko free tier is 30 calls/min
	app.Use(limiter.New(limiter.Config{
		Max:        25,
		Expiration: 1 * time.Minute,
	}))

	// ── Crypto endpoints
	app.Get("/v1/crypto/top", ch.GetTop)
	app.Get("/v1/crypto/price", ch.GetPrices)
	app.Get("/v1/crypto/trending", ch.GetTrending)
	app.Get("/v1/crypto/coin/:id", ch.GetCoinDetail)

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
			"service":       "crypto-api",
			"totalRequests": totalRequests.Load(),
			"totalErrors":   totalErrors.Load(),
			"uptime":        time.Since(startTime).String(),
		})
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "crypto-api", "port": port})
	})

	log.Info().Str("port", port).Msg("crypto-api starting")
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
