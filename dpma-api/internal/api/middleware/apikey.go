package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"time"
)

// APIKey validates the X-RapidAPI-Key, X-API-Key, or ?apikey= parameter.
// skippaths are matched as prefix (e.g. "/health").
func APIKey(keys []string, skipPaths ...string) fiber.Handler {
	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		if k != "" {
			keySet[k] = true
		}
	}
	return func(c *fiber.Ctx) error {
		path := c.Path()
		for _, skip := range skipPaths {
			if strings.HasPrefix(path, skip) {
				return c.Next()
			}
		}
		// accept key from multiple header formats + query param
		key := c.Get("X-RapidAPI-Key")
		if key == "" {
			key = c.Get("X-API-Key")
		}
		if key == "" {
			key = c.Get("Authorization")
			key = strings.TrimPrefix(key, "Bearer ")
			key = strings.TrimPrefix(key, "ApiKey ")
		}
		if key == "" {
			key = c.Query("apikey")
		}
		if !keySet[key] {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or missing API key (X-API-Key header or ?apikey= parameter)",
			})
		}
		return c.Next()
	}
}

// RateLimit limits requests per window per API key.
func RateLimit(max int, window time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: window,
		KeyGenerator: func(c *fiber.Ctx) string {
			key := c.Get("X-RapidAPI-Key")
			if key == "" {
				key = c.Get("X-API-Key")
			}
			if key == "" {
				key = c.IP()
			}
			return key
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
				"limit": max,
				"window": window.String(),
			})
		},
	})
}
