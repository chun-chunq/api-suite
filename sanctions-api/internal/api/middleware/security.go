package middleware

import (
	"strings"
	"unicode"

	"github.com/gofiber/fiber/v2"
)

const maxParamLen = 512

func sanitizeParam(s string) string {
	if len(s) > maxParamLen {
		s = s[:maxParamLen]
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '.' || r == ',' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// SanitizeInputs sanitizes all query parameters.
func SanitizeInputs() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			c.Request().URI().QueryArgs().Set(string(key), sanitizeParam(string(value)))
		})
		return c.Next()
	}
}

// SecurityHeaders adds security response headers.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Content-Security-Policy", "default-src 'none'")
		c.Set("X-Data-Source", "EU Consolidated Sanctions List (FSF) — European Commission")
		c.Set("X-Data-Legal-Basis", "EU Regulation 2580/2001 and subsequent amendments — public compliance data")
		return c.Next()
	}
}
