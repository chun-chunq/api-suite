package main

import (
	"os"
	"sync/atomic"
	"time"

	"aviation-api/internal/api/handler"
	"aviation-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	startTime     = time.Now()
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := log.With().Str("service", "aviation-api").Logger()

	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "changeme"
	}
	// Optional OpenSky account for higher rate limits
	oskyUser := os.Getenv("OPENSKY_USERNAME")
	oskyPass := os.Getenv("OPENSKY_PASSWORD")

	c := client.New(oskyUser, oskyPass)
	aviationH := handler.NewAviationHandler(c, logger)
	mcpH := handler.NewMCPHandler(c, logger)

	app := fiber.New(fiber.Config{
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
	})
	app.Use(recover.New())

	app.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		totalRequests.Add(1)
		if c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "aviation-api", "port": 8100})
	})

	// OpenSky anonymous: 400 API credits/day; enforce low rate limit to be polite
	rl := limiter.New(limiter.Config{
		Max:        30,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			if k := c.Get("X-API-Key"); k != "" { return k }
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "rate limit exceeded — 30 req/min (OpenSky upstream throttle)"})
		},
	})

	v1 := app.Group("/v1", rl)
	v1.Get("/aviation/states", aviationH.GetStates)
	v1.Get("/aviation/aircraft/:icao24/flights", aviationH.GetFlights)
	v1.Get("/aviation/aircraft/:icao24", aviationH.GetAircraft)

	app.Post("/mcp", mcpH.Handle)

	adm := app.Group("/admin", adminAuth(adminSecret))
	adm.Get("/analytics", func(c *fiber.Ctx) error {
		reqs := totalRequests.Load()
		errs := totalErrors.Load()
		rate := 0.0
		if reqs > 0 { rate = float64(errs) / float64(reqs) * 100 }
		return c.JSON(fiber.Map{
			"totalRequests": reqs, "totalErrors": errs,
			"errorRatePct":  rate,
			"uptimeSeconds": int(time.Since(startTime).Seconds()),
			"service":       "aviation-api",
		})
	})

	logger.Info().Msg("aviation-api listening on :8100")
	if err := app.Listen(":8100"); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}

func adminAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Get("X-Admin-Secret") != secret {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
}
