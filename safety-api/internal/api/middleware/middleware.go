package middleware

import (
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"
)

// APIKey validates X-API-Key header; exempt prefixes bypass the check.
func APIKey(validKeys []string, exempt ...string) fiber.Handler {
	set := make(map[string]struct{}, len(validKeys))
	for _, k := range validKeys {
		set[k] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		path := c.Path()
		for _, p := range exempt {
			if strings.HasPrefix(path, p) {
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

// RateLimit limits requests per API key.
type bucket struct {
	count   int
	resetAt time.Time
}

var (
	rlMu     sync.Mutex
	rlBuckets = make(map[string]*bucket)
)

func RateLimit(max int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("X-API-Key")
		if key == "" {
			return c.Next()
		}
		now := time.Now()
		rlMu.Lock()
		b := rlBuckets[key]
		if b == nil || now.After(b.resetAt) {
			b = &bucket{resetAt: now.Add(window)}
			rlBuckets[key] = b
		}
		b.count++
		count := b.count
		rlMu.Unlock()
		if count > max {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "rate limit exceeded"})
		}
		return c.Next()
	}
}

// SanitizeInputs strips dangerous characters from query params.
func SanitizeInputs() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
			c.Request().URI().QueryArgs().Set(string(key), sanitize(string(value)))
		})
		return c.Next()
	}
}

func sanitize(s string) string {
	if len(s) > 512 {
		s = s[:512]
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == ' ' || r == '-' || r == '.' || r == ',' || r == '/' || r == '_' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// SecurityHeaders adds standard security response headers.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("X-Data-Source", "EU Safety Gate — European Commission DG JUST")
		c.Set("X-Data-Legal-Basis", "EU General Product Safety Regulation — public safety data")
		return c.Next()
	}
}
