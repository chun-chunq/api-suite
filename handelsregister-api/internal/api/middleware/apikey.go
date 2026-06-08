package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/handelsregister-api/internal/cache"
)

// apiKeyContextKey is the locals key under which the validated key is stored.
const apiKeyContextKey = "apiKey"

// APIKeyAuth validates the X-API-Key header (or `apikey` query param as a
// RapidAPI-friendly fallback) against the configured key set, and meters usage.
func APIKeyAuth(validKeys map[string]bool, c *cache.Cache, log zerolog.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		key := ctx.Get("X-API-Key")
		if key == "" {
			key = ctx.Get("X-RapidAPI-Key")
		}
		if key == "" {
			key = ctx.Query("apikey")
		}

		if key == "" {
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing API key",
				"hint":  "send your key via the X-API-Key header",
			})
		}

		if !validKeys[key] {
			log.Warn().Str("ip", ctx.IP()).Msg("rejected invalid API key")
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid API key",
			})
		}

		ctx.Locals(apiKeyContextKey, key)

		// Meter usage for billing prep. Failure to record must not block the
		// request, but it is logged for observability.
		if count, err := c.IncrUsage(ctx.UserContext(), key); err != nil {
			log.Error().Err(err).Msg("usage metering failed")
		} else {
			ctx.Set("X-Usage-Today", itoa(count))
		}

		return ctx.Next()
	}
}

// APIKeyFromCtx returns the validated key stored by APIKeyAuth.
func APIKeyFromCtx(ctx *fiber.Ctx) string {
	if v, ok := ctx.Locals(apiKeyContextKey).(string); ok {
		return v
	}
	return ""
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
