package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/handelsregister-api/config"
	"github.com/handelsregister-api/internal/api/handler"
	"github.com/handelsregister-api/internal/api/middleware"
	"github.com/handelsregister-api/internal/cache"
	"github.com/handelsregister-api/internal/scraper"
)

// Deps bundles everything the router needs to wire handlers.
type Deps struct {
	Config  *config.Config
	Cache   *cache.Cache
	Scraper *scraper.Scraper
	Queue   *asynq.Client
	Logger  zerolog.Logger
	Version string
}

// NewRouter builds the configured Fiber application.
func NewRouter(d Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "handelsregister-api " + d.Version,
		DisableStartupMessage: true,
		ErrorHandler:          errorHandler(d.Logger),
	})

	app.Use(recover.New())
	app.Use(requestLogger(d.Logger))

	healthH := handler.NewHealthHandler(d.Cache, d.Version)
	companyH := handler.NewCompanyHandler(d.Cache, d.Scraper, d.Queue, d.Logger)

	// Public, unauthenticated.
	app.Get("/health", healthH.Health)

	// Versioned, authenticated + rate-limited API surface.
	v1 := app.Group("/v1")
	v1.Use(middleware.APIKeyAuth(d.Config.APIKeys, d.Cache, d.Logger))
	v1.Use(middleware.RateLimit(d.Cache.Client(), d.Config.RateLimitPerMinute, d.Logger))

	company := v1.Group("/company")
	// Note: register the static "search" route before the dynamic ":hrb"
	// route so /v1/company/search is not captured as an HRB value.
	company.Get("/search", companyH.Search)
	company.Get("/:hrb", companyH.GetByHRB)

	app.Use(notFound)

	return app
}

func errorHandler(log zerolog.Logger) fiber.ErrorHandler {
	return func(ctx *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		var fe *fiber.Error
		if e, ok := err.(*fiber.Error); ok {
			fe = e
			code = e.Code
		}
		if code >= 500 {
			log.Error().Err(err).Str("path", ctx.Path()).Msg("request error")
		}
		msg := "internal server error"
		if fe != nil {
			msg = fe.Message
		}
		return ctx.Status(code).JSON(fiber.Map{"error": msg})
	}
}

func requestLogger(log zerolog.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		start := time.Now()
		err := ctx.Next()
		log.Info().
			Str("method", ctx.Method()).
			Str("path", ctx.Path()).
			Int("status", ctx.Response().StatusCode()).
			Dur("latency", time.Since(start)).
			Str("ip", ctx.IP()).
			Msg("request")
		return err
	}
}

func notFound(ctx *fiber.Ctx) error {
	return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
		"error": "route not found",
		"path":  ctx.Path(),
	})
}
