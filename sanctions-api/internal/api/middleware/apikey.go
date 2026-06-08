package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

// APIKey checks for a valid X-API-Key header. Exempt path prefixes bypass the check.
func APIKey(validKeys []string, exempt ...string) fiber.Handler {
	set := make(map[string]struct{}, len(validKeys))
	for _, k := range validKeys {
		set[k] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		path := c.Path()
		for _, prefix := range exempt {
			if strings.HasPrefix(path, prefix) {
				return c.Next()
			}
		}
		key := c.Get("X-API-Key")
		if key == "" {
			key = c.Get("X-RapidAPI-Proxy-Secret")
		}
		if _, ok := set[key]; !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing or invalid API key — set X-API-Key header",
			})
		}
		return c.Next()
	}
}
