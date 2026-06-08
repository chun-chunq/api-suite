package main

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gleif-api/internal/api/handler"
	"github.com/gleif-api/internal/api/middleware"
	"github.com/gleif-api/internal/client"
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
	// Logging
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("LOG_LEVEL") == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8089"
	}

	gleifClient := client.New()
	leiHandler := handler.NewLEIHandler(gleifClient)

	app := fiber.New(fiber.Config{
		AppName:      "GLEIF LEI API v1",
		ReadTimeout:  20 * time.Second,
		WriteTimeout: 20 * time.Second,
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
				"error":       "rate limit exceeded",
				"retryAfter":  "3600s",
			})
		},
	}))
	app.Use(middleware.Auth())

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
				"service":       "gleif-api",
			})
		})
	}

	// Request counter middleware (before auth so health is also counted)
	app.Use(func(c *fiber.Ctx) error {
		err := c.Next()
		totalRequests.Add(1)
		if c.Response().StatusCode() >= 500 {
			totalErrors.Add(1)
		}
		return err
	})

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":     "ok",
			"service":    "gleif-lei-api",
			"dataSource": "GLEIF Global LEI Index",
			"apiUrl":     "https://api.gleif.org/api/v1/",
		})
	})

	// LEI endpoints
	v1 := app.Group("/v1")
	lei := v1.Group("/lei")
	lei.Get("/search", leiHandler.Search)
	lei.Get("/:lei", leiHandler.GetByLEI)
	lei.Get("/:lei/relationships", leiHandler.GetRelationships)

	// MCP (Model Context Protocol) — AI assistant integration
	mcpHandler := handler.NewMCPHandler(gleifClient)
	app.Post("/mcp", mcpHandler.Handle)

	// Root
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": "GLEIF LEI API",
			"version": "1.0.0",
			"description": "Search 2.3M+ legal entities by LEI code or company name. Data from the Global LEI Index (GLEIF). Required for MiFID II, EMIR, Dodd-Frank reporting.",
			"endpoints": []fiber.Map{
				{"GET /v1/lei/search": "Search by company name", "params": "name (required), country (ISO-2), active (bool), limit (1-200)"},
				{"GET /v1/lei/:lei": "Lookup by 20-char LEI code"},
				{"GET /v1/lei/:lei/relationships": "Parent/child ownership structure"},
				{"GET /health": "Health check"},
			},
			"exampleCalls": []string{
				"/v1/lei/search?name=Deutsche+Bank&country=DE&active=true",
				"/v1/lei/5299000J2N45DDNE4Y28",
				"/v1/lei/5299000J2N45DDNE4Y28/relationships",
			},
			"dataSource": "GLEIF Global LEI Index — https://www.gleif.org/",
			"coverage":   "2.3M+ legal entities worldwide",
		})
	})

	log.Info().Str("port", port).Msg("GLEIF LEI API starting")
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}
