package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// APIKey checks for a valid X-API-Key header. Exempt paths bypass the check.
func APIKey(validKeys []string, exempt ...string) fiber.Handler {
	set := make(map[string]struct{}, len(validKeys))
	for _, k := range validKeys {
		set[k] = struct{}{}
	}
	exemptSet := make(map[string]struct{}, len(exempt))
	for _, p := range exempt {
		exemptSet[p] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		if _, ok := exemptSet[c.Path()]; ok {
			return c.Next()
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
