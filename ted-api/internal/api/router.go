package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"

	"github.com/ted-api/config"
	"github.com/ted-api/internal/api/handler"
	"github.com/ted-api/internal/api/middleware"
	"github.com/ted-api/internal/cache"
)

func NewRouter(cfg *config.Config, c *cache.Cache, log zerolog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "ted-api",
		DisableStartupMessage: true,
		ReadTimeout:           35 * time.Second,
		WriteTimeout:          35 * time.Second,
	})

	app.Use(recover.New())
	app.Use(cors.New())
	app.Use(requestLogger(log))
	app.Use(middleware.LegalHeaders())

	health := handler.NewHealthHandler(c)
	app.Get("/health", health.Health)

	app.Use(middleware.APIKey(cfg.APIKeys, "/health"))
	app.Use(middleware.RateLimit(cfg.RateLimitMax, cfg.RateLimitWindow))

	notices := handler.NewNoticesHandler(c, log, cfg.CacheTTL)
	v1 := app.Group("/v1/ted")
	v1.Get("/search", notices.Search)
	v1.Get("/recent", notices.Recent)
	v1.Get("/notice-types", notices.NoticeTypes)

	return app
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
