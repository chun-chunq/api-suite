package main

import (
	"os"
	"sync/atomic"
	"time"

	"uk-company-api/internal/api/handler"
	"uk-company-api/internal/client"

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
	logger := log.With().Str("service", "uk-company-api").Logger()

	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "changeme"
	}
	// Companies House requires a free API key
	// Get one at: https://developer.company-information.service.gov.uk/
	chAPIKey := os.Getenv("COMPANIES_HOUSE_API_KEY")
	if chAPIKey == "" {
		logger.Warn().Msg("COMPANIES_HOUSE_API_KEY not set — Companies House requests will fail auth")
	}

	c := client.New(chAPIKey)
	companyH := handler.NewCompanyHandler(c, logger)
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
		return c.JSON(fiber.Map{"status": "ok", "service": "uk-company-api", "port": 8095})
	})

	rl := limiter.New(limiter.Config{
		Max:        100,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			if k := c.Get("X-API-Key"); k != "" {
				return k
			}
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "rate limit exceeded — 100 req/min"})
		},
	})

	v1 := app.Group("/v1", rl)
	v1.Get("/uk/company/search", companyH.Search)
	v1.Get("/uk/company/:number/officers", companyH.GetOfficers)
	v1.Get("/uk/company/:number", companyH.GetByNumber)

	app.Post("/mcp", mcpH.Handle)

	adm := app.Group("/admin", adminAuth(adminSecret))
	adm.Get("/analytics", func(c *fiber.Ctx) error {
		reqs := totalRequests.Load()
		errs := totalErrors.Load()
		rate := 0.0
		if reqs > 0 {
			rate = float64(errs) / float64(reqs) * 100
		}
		return c.JSON(fiber.Map{
			"totalRequests": reqs,
			"totalErrors":   errs,
			"errorRatePct":  rate,
			"uptimeSeconds": int(time.Since(startTime).Seconds()),
			"service":       "uk-company-api",
		})
	})

	logger.Info().Msg("uk-company-api listening on :8095")
	if err := app.Listen(":8095"); err != nil {
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
