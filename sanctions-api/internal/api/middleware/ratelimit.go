package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type bucket struct {
	count  int
	resetAt time.Time
}

var (
	rlMu      sync.Mutex
	rlBuckets = make(map[string]*bucket)
)

// RateLimit limits requests per API key to max requests per window.
func RateLimit(max int, window time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.Get("X-API-Key")
		if key == "" {
			key = c.Get("X-RapidAPI-Proxy-Secret")
		}
		if key == "" {
			return c.Next() // no key = no limit (already blocked by APIKey middleware)
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
		resetAt := b.resetAt
		rlMu.Unlock()

		if count > max {
			c.Set("Retry-After", time.Until(resetAt).String())
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		}
		return c.Next()
	}
}
