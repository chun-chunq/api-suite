package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimit returns a fixed-window rate limiter keyed by API key (falling back to
// client IP). It uses Fiber's in-memory store; for multi-instance deployments a
// Redis-backed store can be substituted via limiter.Config.Storage.
func RateLimit(max int, window time.Duration) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: window,
		KeyGenerator: func(c *fiber.Ctx) string {
			if key := c.Get("X-API-Key"); key != "" {
				return key
			}
			if key := c.Get("X-RapidAPI-Proxy-Secret"); key != "" {
				return key
			}
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded, slow down",
			})
		},
	})
}
