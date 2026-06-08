package middleware

import (
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// BandwidthTracker tracks monthly outbound bytes and logs warnings.
// Hetzner VPS plans include 20TB/month — we warn at the configurable limit.
type BandwidthTracker struct {
	bytesThisMonth atomic.Int64
	limitBytes     int64 // 0 = disabled
	resetDay       int   // day-of-month to reset (always 1)
	log            zerolog.Logger
	lastReset      time.Time
}

// NewBandwidthTracker creates a tracker with a soft monthly limit in GB.
// Set limitGB=0 to disable blocking (still tracks).
func NewBandwidthTracker(limitGB int64, log zerolog.Logger) *BandwidthTracker {
	t := &BandwidthTracker{
		limitBytes: limitGB * 1024 * 1024 * 1024,
		log:        log,
		lastReset:  time.Now(),
	}
	return t
}

// Middleware returns a Fiber middleware that counts outbound bytes.
func (bt *BandwidthTracker) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		bt.maybeReset()

		// Check limit before processing
		if bt.limitBytes > 0 {
			used := bt.bytesThisMonth.Load()
			if used >= bt.limitBytes {
				bt.log.Error().
					Int64("usedGB", used/1024/1024/1024).
					Int64("limitGB", bt.limitBytes/1024/1024/1024).
					Msg("monthly bandwidth limit reached — refusing request")
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"error": "monthly bandwidth limit reached; resets on the 1st of next month",
					"code":  "BANDWIDTH_LIMIT",
				})
			}
			// Warn at 80%
			if used > bt.limitBytes*80/100 {
				bt.log.Warn().
					Int64("usedGB", used/1024/1024/1024).
					Int64("limitGB", bt.limitBytes/1024/1024/1024).
					Msg("bandwidth usage above 80% of monthly limit")
			}
		}

		err := c.Next()

		// Count response body size
		n := int64(len(c.Response().Body()))
		bt.bytesThisMonth.Add(n)

		return err
	}
}

// Stats returns current usage stats.
func (bt *BandwidthTracker) Stats() map[string]any {
	used := bt.bytesThisMonth.Load()
	m := map[string]any{
		"usedBytes":  used,
		"usedGB":     float64(used) / 1024 / 1024 / 1024,
		"resetAt":    bt.lastReset.AddDate(0, 1, 0).Format("2006-01-02"),
	}
	if bt.limitBytes > 0 {
		m["limitGB"] = float64(bt.limitBytes) / 1024 / 1024 / 1024
		m["usedPercent"] = float64(used) * 100 / float64(bt.limitBytes)
	}
	return m
}

func (bt *BandwidthTracker) maybeReset() {
	now := time.Now()
	if now.Month() != bt.lastReset.Month() || now.Year() != bt.lastReset.Year() {
		bt.bytesThisMonth.Store(0)
		bt.lastReset = now
		bt.log.Info().Msg("monthly bandwidth counter reset")
	}
}
