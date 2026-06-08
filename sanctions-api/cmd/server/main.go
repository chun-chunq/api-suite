package main

import (
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/sanctions-api/config"
	"github.com/sanctions-api/internal/api/handler"
	"github.com/sanctions-api/internal/api/middleware"
	"github.com/sanctions-api/internal/sanctions"
)

func main() {
	cfg := config.Load()

	// Logger
	level, _ := zerolog.ParseLevel(cfg.LogLevel)
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger().Level(level)
	log.Logger = logger

	logger.Info().Str("port", cfg.Port).Msg("starting sanctions-api")

	// Load EU sanctions list (blocks until first fetch; retries in background)
	logger.Info().Msg("loading EU consolidated sanctions list…")
	idx := sanctions.New(cfg.RefreshInterval, logger)

	// HTTP server
	app := fiber.New(fiber.Config{
		AppName:               "sanctions-api",
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
		return c.JSON(fiber.Map{
			"status":    status,
			"sanctions": s,
		})
	})

	// Admin — protected by X-Admin-Secret
	if cfg.AdminSecret != "" {
		adm := app.Group("/admin", adminAuth(cfg.AdminSecret))
		adm.Get("/status", func(c *fiber.Ctx) error {
			return c.JSON(idx.Status())
		})
	}

	// API routes — protected by X-API-Key
	app.Use(middleware.APIKey(cfg.APIKeys, "/health", "/admin/"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))

	h := handler.NewSanctionsHandler(idx, logger)
	v1 := app.Group("/v1/sanctions")
	v1.Get("/search", h.Search)  // full search with details
	v1.Get("/check", h.Check)    // quick yes/no check
	v1.Get("/status", h.Status)  // list health / last update date

	// MCP (Model Context Protocol) — AI assistant integration
	mcp := handler.NewMCPHandler(idx, logger)
	app.Post("/mcp", mcp.Handle)

	logger.Info().
		Str("port", cfg.Port).
		Int("rateLimitPerHour", cfg.RateLimitMax).
		Msg("sanctions-api ready")

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
