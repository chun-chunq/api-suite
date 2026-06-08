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

	"github.com/bafin-api/config"
	"github.com/bafin-api/internal/api/handler"
	"github.com/bafin-api/internal/api/middleware"
	"github.com/bafin-api/internal/cache"
	"github.com/bafin-api/internal/scrapequeue"
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

	logger.Info().Str("port", cfg.Port).Msg("starting bafin-api (BaFin licensed institutions)")

	// Cache (optional)
	var c *cache.Cache
	if cfg.RedisAddr != "" {
		var err error
		c, err = cache.New(cfg.RedisAddr)
		if err != nil {
			logger.Warn().Err(err).Msg("Redis unavailable — caching disabled")
		}
	}

	q := scrapequeue.New(cfg.MaxBrowsers, cfg.MaxQueueDepth)
	logger.Info().Int("slots", cfg.MaxBrowsers).Int("maxDepth", cfg.MaxQueueDepth).Msg("scrape queue configured")

	app := fiber.New(fiber.Config{
		AppName:               "bafin-api",
		DisableStartupMessage: true,
		ReadTimeout:           70 * time.Second,
		WriteTimeout:          70 * time.Second,
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(requestLogger(logger))
	app.Use(middleware.SecurityHeaders())
	app.Use(middleware.SanitizeInputs())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"queue":  q.Stats(),
		})
	})

	if cfg.AdminSecret != "" {
		adm := app.Group("/admin", adminAuth(cfg.AdminSecret))
		adm.Get("/stats", func(ctx *fiber.Ctx) error {
			return ctx.JSON(fiber.Map{"queue": q.Stats()})
		})
		adm.Get("/analytics", func(ctx *fiber.Ctx) error {
			reqs := totalRequests.Load()
			errs := totalErrors.Load()
			errRate := 0.0
			if reqs > 0 {
				errRate = float64(errs) / float64(reqs) * 100
			}
			return ctx.JSON(fiber.Map{
				"totalRequests":  reqs,
				"totalErrors":    errs,
				"errorRatePct":   errRate,
				"uptimeSeconds":  int(time.Since(startTime).Seconds()),
				"queueStats":     q.Stats(),
				"service":        "bafin-api",
			})
		})
	}

	app.Use(middleware.APIKey(cfg.APIKeys, "/health", "/admin/"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))

	app.Use(func(ctx *fiber.Ctx) error {
		err := ctx.Next()
		totalRequests.Add(1)
		if ctx.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	h := handler.NewInstitutionHandler(c, logger, cfg.CacheTTL, cfg.ChromeBin, q)
	v1 := app.Group("/v1/bafin")
	v1.Get("/search", h.Search)
	v1.Get("/license-types", h.LicenseTypes)

	logger.Info().Str("port", cfg.Port).
		Msg("bafin-api ready — endpoints: /v1/bafin/{search, license-types}")

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
