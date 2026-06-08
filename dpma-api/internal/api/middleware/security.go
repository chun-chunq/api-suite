// Package middleware provides shared HTTP middleware for the API.
// Security note: all user inputs are sanitized here before reaching handlers.
package middleware

import (
	"strings"
	"unicode"

	"github.com/gofiber/fiber/v2"
)

// maxParamLen is the hard limit on any single query parameter value.
const maxParamLen = 512

// allowedRunes returns true for characters we accept in search params.
// Allows letters, digits, spaces, and common punctuation used in company/trademark names.
var allowedParamChars = buildAllowedSet()

func buildAllowedSet() map[rune]bool {
	allowed := map[rune]bool{}
	extra := " .,;:-()&+/äöüÄÖÜß"
	for _, r := range extra {
		allowed[r] = true
	}
	return allowed
}

// sanitizeParam strips control characters, null bytes, and overly long values.
// It preserves letters, digits, and common search punctuation.
func sanitizeParam(s string) string {
	if len(s) > maxParamLen {
		s = s[:maxParamLen]
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || allowedParamChars[r] {
			b.WriteRune(r)
		}
		// silently drop control chars, null bytes, JS/HTML injection chars
	}
	return strings.TrimSpace(b.String())
}

// SanitizeInputs validates and sanitizes all query parameters.
// This prevents JS injection into go-rod eval calls and log injection.
func SanitizeInputs() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			clean := sanitizeParam(string(value))
			c.Request().URI().QueryArgs().Set(string(key), clean)
		})
		return c.Next()
	}
}

// SecurityHeaders adds security-relevant HTTP headers to every response.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Content-Security-Policy", "default-src 'none'")
		return c.Next()
	}
}

// LegalHeaders adds GDPR and data source headers.
func LegalHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Data-Source", "DPMA Register (register.dpma.de) — public trademark register")
		c.Set("X-Data-Legal-Basis", "DSGVO Art. 6(1)(f) — berechtigtes Interesse; öffentliches Register §§ 25 ff. MarkenG")
		c.Set("X-Data-Usage-Restriction", "B2B trademark clearance and brand monitoring only; no bulk harvesting")
		return c.Next()
	}
}
