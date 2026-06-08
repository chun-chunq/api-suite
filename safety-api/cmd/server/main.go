package main

import (
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/safety-api/config"
	"github.com/safety-api/internal/api/handler"
	"github.com/safety-api/internal/api/middleware"
	"github.com/safety-api/internal/safety"
)

var (
	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	startTime     = time.Now()
)

func main() {
	cfg := config.Load()

	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger().Level(level)
	log.Logger = logger

	logger.Info().Str("port", cfg.Port).Msg("starting safety-api (EU Safety Gate)")

	// Load EU Safety Gate data — blocks until done or fails gracefully
	idx := safety.New(cfg.RefreshInterval, logger)

	app := fiber.New(fiber.Config{
		AppName:               "safety-api",
		DisableStartupMessage: true,
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          10 * time.Second,
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(requestLogger(logger))
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.SanitizeInputs())

	// Health — no auth
	app.Get("/health", func(c *fiber.Ctx) error {
		s := idx.Status()
		status := "ok"
		if !s.Loaded {
			status = "loading"
		}
		return c.JSON(fiber.Map{"status": status, "data": s})
	})

	if cfg.AdminSecret != "" {
		adm := app.Group("/admin", adminAuth(cfg.AdminSecret))
		adm.Get("/status", func(c *fiber.Ctx) error { return c.JSON(idx.Status()) })
		adm.Get("/analytics", func(c *fiber.Ctx) error {
			reqs := totalRequests.Load()
			errs := totalErrors.Load()
			errRate := 0.0
			if reqs > 0 {
				errRate = float64(errs) / float64(reqs) * 100
			}
			return c.JSON(fiber.Map{
				"totalRequests": reqs,
				"totalErrors":   errs,
				"errorRatePct":  errRate,
				"uptimeSeconds": int(time.Since(startTime).Seconds()),
				"dataStatus":    idx.Status(),
				"service":       "safety-api",
			})
		})
	}

	app.Use(middleware.APIKey(cfg.APIKeys, "/health", "/admin/"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))
	app.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		totalRequests.Add(1)
		if c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	h := handler.NewAlertsHandler(idx, logger)
	v1 := app.Group("/v1/recalls")
	v1.Get("/search", h.Search)
	v1.Get("/categories", h.Categories)
	v1.Get("/status", h.Status)
	v1.Get("/:reference", h.Get) // must be last

	logger.Info().
		Str("port", cfg.Port).
		Msg("safety-api ready — endpoints: /v1/recalls/{search,categories,status,:reference}")

	if err := app.Listen(":" + cfg.Port); err != nil {
		logger.Fatal().Err(err).Msg("server error")
	}
}

func adminAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("X-Admin-Secret")
		if key == "" {
			key = strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		}
		if key != secret {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
}

func requestLogger(log zerolog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		log.Info().
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).
			Dur("latency", time.Since(start)).
			Msg("request")
		return err
	}
}
