package main

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/zefix-api/config"
	"github.com/zefix-api/internal/api/handler"
	"github.com/zefix-api/internal/api/middleware"
	"github.com/zefix-api/internal/zefix"
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

	logger.Info().Str("port", cfg.Port).Msg("starting zefix-api (Swiss company register)")

	// Redis for caching
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		logger.Warn().Err(err).Msg("Redis unavailable — caching disabled")
		rdb = nil
	}

	client := zefix.New()

	app := fiber.New(fiber.Config{
		AppName:               "zefix-api",
		DisableStartupMessage: true,
		ReadTimeout:           15 * time.Second,
		WriteTimeout:          15 * time.Second,
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(requestLogger(logger))
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.SanitizeInputs())

	app.Get("/health", func(c *fiber.Ctx) error {
		redisOk := rdb != nil && rdb.Ping(c.Context()).Err() == nil
		return c.JSON(fiber.Map{"status": "ok", "redis": redisOk})
	})

	if cfg.AdminSecret != "" {
		adm := app.Group("/admin", adminAuth(cfg.AdminSecret))
		adm.Get("/status", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"status": "ok", "dataSource": "Zefix official REST API"})
		})
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
				"service":       "zefix-api",
			})
		})
	}

	app.Use(middleware.APIKey(cfg.APIKeys, "/health", "/admin/"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))

	// Request counter middleware
	app.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		totalRequests.Add(1)
		if c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	h := handler.NewCompanyHandler(client, rdb, cfg.CacheTTL, logger)
	v1 := app.Group("/v1/ch")
	v1.Get("/company/search", h.Search)
	v1.Get("/company/:uid/publications", h.GetPublications)
	v1.Get("/company/:uid", h.GetByUID)

	logger.Info().Str("port", cfg.Port).
		Msg("zefix-api ready — endpoints: /v1/ch/company/{search, :uid, :uid/publications}")

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
		log.Info().Str("method", c.Method()).Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).Dur("latency", time.Since(start)).Msg("request")
		return err
	}
}
