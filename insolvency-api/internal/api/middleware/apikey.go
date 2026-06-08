package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// APIKey returns middleware that enforces presence of a valid API key. Keys may
// be supplied via the `X-API-Key` header or the `X-RapidAPI-Proxy-Secret`
// header (used when fronted by RapidAPI). Requests on whitelisted paths bypass
// the check.
func APIKey(validKeys []string, bypass ...string) fiber.Handler {
	keySet := make(map[string]struct{}, len(validKeys))
	for _, k := range validKeys {
		keySet[k] = struct{}{}
	}
	bypassSet := make(map[string]struct{}, len(bypass))
	for _, p := range bypass {
		bypassSet[p] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		if _, ok := bypassSet[c.Path()]; ok {
			return c.Next()
		}

		key := c.Get("X-API-Key")
		if key == "" {
			key = c.Get("X-RapidAPI-Proxy-Secret")
		}
		if key == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing API key (provide X-API-Key header)",
			})
		}
		if _, ok := keySet[key]; !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "invalid API key",
			})
		}
		return c.Next()
	}
}
