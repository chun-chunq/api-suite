package middleware

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// RateLimit enforces a per-API-key fixed-window limit using Redis counters.
// It must run after APIKeyAuth so a validated key is present in locals.
func RateLimit(rdb *redis.Client, perMinute int, log zerolog.Logger) fiber.Handler {
	if perMinute <= 0 {
		perMinute = 60
	}
	window := time.Minute

	return func(ctx *fiber.Ctx) error {
		key := APIKeyFromCtx(ctx)
		if key == "" {
			// No authenticated key (e.g. misordered middleware); fall back to IP.
			key = ctx.IP()
		}

		bucket := time.Now().UTC().Truncate(window).Unix()
		rkey := fmt.Sprintf("ratelimit:%s:%d", key, bucket)

		uctx := ctx.UserContext()
		pipe := rdb.TxPipeline()
		incr := pipe.Incr(uctx, rkey)
		pipe.Expire(uctx, rkey, window+time.Second)
		if _, err := pipe.Exec(uctx); err != nil {
			// Fail open: if Redis is unreachable we should not hard-block paying
			// customers, but we log loudly.
			log.Error().Err(err).Msg("rate limit backend error, failing open")
			return ctx.Next()
		}

		count := incr.Val()
		remaining := int64(perMinute) - count
		if remaining < 0 {
			remaining = 0
		}

		ctx.Set("X-RateLimit-Limit", itoa(int64(perMinute)))
		ctx.Set("X-RateLimit-Remaining", itoa(remaining))
		ctx.Set("X-RateLimit-Reset", itoa(bucket+int64(window.Seconds())))

		if count > int64(perMinute) {
			retryAfter := int64(window.Seconds()) - (time.Now().UTC().Unix() - bucket)
			ctx.Set("Retry-After", itoa(retryAfter))
			return ctx.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "rate limit exceeded",
				"limit":       perMinute,
				"retry_after": retryAfter,
			})
		}

		return ctx.Next()
	}
}
