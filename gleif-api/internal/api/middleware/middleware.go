package middleware

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Auth validates X-API-Key or X-RapidAPI-Proxy-Secret header.
func Auth() fiber.Handler {
	rawKeys := os.Getenv("API_KEYS")
	validKeys := map[string]bool{}
	for _, k := range strings.Split(rawKeys, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			if idx := strings.Index(k, ":"); idx != -1 {
				k = k[:idx]
			}
			validKeys[k] = true
		}
	}
	return func(c *fiber.Ctx) error {
		// Skip auth for health and admin
		p := c.Path()
		if p == "/health" || strings.HasPrefix(p, "/admin") {
			return c.Next()
		}
		key := c.Get("X-API-Key")
		if key == "" {
			key = c.Get("X-RapidAPI-Proxy-Secret")
		}
		if len(validKeys) == 0 || validKeys[key] {
			return c.Next()
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "invalid or missing API key",
		})
	}
}

// SecurityHeaders adds standard security response headers.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		return c.Next()
	}
}
