package main

import (
	"os"
	"sync/atomic"
	"time"

	"sec-api/internal/api/handler"
	"sec-api/internal/client"

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
	logger := log.With().Str("service", "sec-api").Logger()

	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		adminSecret = "changeme"
	}
	contactEmail := os.Getenv("CONTACT_EMAIL")
	userAgent := "SEC-API " + contactEmail

	c := client.New(userAgent)
	edgarH := handler.NewEdgarHandler(c, logger)
	mcpH := handler.NewMCPHandler(c, logger)

	app := fiber.New(fiber.Config{
		ReadTimeout:  25 * time.Second,
		WriteTimeout: 25 * time.Second,
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
		return c.JSON(fiber.Map{"status": "ok", "service": "sec-api", "port": 8098})
	})

	// SEC asks for polite usage — limit to 60/min to stay well below their 600/min limit
	rl := limiter.New(limiter.Config{
		Max:        60,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			if k := c.Get("X-API-Key"); k != "" { return k }
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(429).JSON(fiber.Map{"error": "rate limit exceeded — 60 req/min"})
		},
	})

	v1 := app.Group("/v1", rl)
	v1.Get("/sec/company/search", edgarH.Search)
	v1.Get("/sec/company/:cik/filings", edgarH.GetFilings)
	v1.Get("/sec/company/:cik/financials", edgarH.GetFinancials)
	v1.Get("/sec/company/:cik", edgarH.GetProfile)

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
			"service":       "sec-api",
		})
	})

	logger.Info().Msg("sec-api listening on :8098")
	if err := app.Listen(":8098"); err != nil {
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
