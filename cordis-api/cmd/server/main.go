package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cordis-api/internal/api/handler"
	"github.com/cordis-api/internal/api/middleware"
	"github.com/cordis-api/internal/client"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	totalRequests atomic.Int64
	totalErrors   atomic.Int64
	svcStartTime  = time.Now()
)

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

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("LOG_LEVEL") == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	cordisClient := client.New()
	grantsHandler := handler.NewGrantsHandler(cordisClient)

	app := fiber.New(fiber.Config{
		AppName:      "CORDIS EU Grants API v1",
		ReadTimeout:  25 * time.Second,
		WriteTimeout: 25 * time.Second,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		},
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{AllowOrigins: "*", AllowHeaders: "X-API-Key, X-RapidAPI-Proxy-Secret, Content-Type"}))
	app.Use(middleware.SecurityHeaders())
	app.Use(limiter.New(limiter.Config{
		Max:        200,
		Expiration: time.Hour,
		KeyGenerator: func(c *fiber.Ctx) string {
			key := c.Get("X-API-Key")
			if key == "" {
				key = c.IP()
			}
			return key
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":      "rate limit exceeded",
				"retryAfter": "3600s",
			})
		},
	}))
	app.Use(middleware.Auth())

	// Request counter
	app.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		totalRequests.Add(1)
		if c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	// Admin analytics
	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret != "" {
		adm := app.Group("/admin", adminAuth(adminSecret))
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
				"uptimeSeconds": int(time.Since(svcStartTime).Seconds()),
				"service":       "cordis-api",
			})
		})
	}

	// Health
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "ok",
			"service":    "cordis-grants-api",
			"dataSource": "EU CORDIS — Horizon Europe / H2020 / FP7",
			"apiUrl":     "https://cordis.europa.eu/",
		})
	})

	// Grant endpoints
	v1 := app.Group("/v1")
	grants := v1.Group("/grants")
	grants.Get("/search", grantsHandler.Search)
	grants.Get("/:id", grantsHandler.GetProject)

	// Root
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service":     "CORDIS EU Research Grants API",
			"version":     "1.0.0",
			"description": "Search 50,000+ EU-funded research projects (Horizon Europe, H2020, FP7). Find grants by keyword, country, programme, status. Data from the official EU CORDIS database.",
			"endpoints": []fiber.Map{
				{"GET /v1/grants/search": "Search projects by keyword, country, programme", "params": "q, country, programme, from, to, status, limit, page"},
				{"GET /v1/grants/:id": "Get project details by CORDIS ID"},
				{"GET /health": "Health check"},
			},
			"exampleCalls": []string{
				"/v1/grants/search?q=artificial+intelligence&country=DE&programme=HORIZON",
				"/v1/grants/search?q=climate+change&from=2021&to=2024&status=ACTIVE",
				"/v1/grants/101016775",
			},
			"programmes": []string{"HORIZON (Horizon Europe 2021-2027)", "H2020 (Horizon 2020 2014-2020)", "FP7 (7th Framework 2007-2013)"},
			"dataSource": "EU CORDIS — https://cordis.europa.eu/",
			"coverage":   "50,000+ projects, €100B+ in grants",
		})
	})

	log.Info().Str("port", port).Msg("CORDIS EU Grants API starting")
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
