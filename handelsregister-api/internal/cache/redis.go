package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMiss indicates the requested key was not present in the cache.
var ErrMiss = errors.New("cache: miss")

// Cache is a thin JSON-serializing wrapper around go-redis.
type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

// New builds a Cache from connection parameters and verifies connectivity.
func New(ctx context.Context, addr, password string, db int, ttl time.Duration) (*Cache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return nil, fmt.Errorf("cache: ping redis: %w", err)
	}

	return &Cache{rdb: rdb, ttl: ttl}, nil
}

// Client exposes the underlying redis client (used for health checks etc.).
func (c *Cache) Client() *redis.Client { return c.rdb }

// Close releases the redis connection pool.
func (c *Cache) Close() error { return c.rdb.Close() }

// GetJSON unmarshals the value at key into dst. Returns ErrMiss on absence.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) error {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrMiss
	}
	if err != nil {
		return fmt.Errorf("cache: get %q: %w", key, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("cache: unmarshal %q: %w", key, err)
	}
	return nil
}

// SetJSON stores value as JSON at key using the configured TTL.
func (c *Cache) SetJSON(ctx context.Context, key string, value any) error {
	return c.SetJSONTTL(ctx, key, value, c.ttl)
}

// SetJSONTTL stores value as JSON with an explicit TTL.
func (c *Cache) SetJSONTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: marshal %q: %w", key, err)
	}
	if err := c.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache: set %q: %w", key, err)
	}
	return nil
}

// Health pings redis to confirm liveness.
func (c *Cache) Health(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// IncrUsage atomically increments a per-API-key usage counter and returns the
// new value. The counter resets on a daily window for billing/metering prep.
func (c *Cache) IncrUsage(ctx context.Context, apiKey string) (int64, error) {
	day := time.Now().UTC().Format("2006-01-02")
	key := fmt.Sprintf("usage:%s:%s", apiKey, day)

	pipe := c.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 48*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("cache: incr usage: %w", err)
	}
	return incr.Val(), nil
}

// CompanyKey builds the canonical cache key for a company lookup.
func CompanyKey(hrb, state string) string {
	return fmt.Sprintf("company:%s:%s", state, hrb)
}

// SearchKey builds the cache key for a name search.
func SearchKey(name string) string {
	return fmt.Sprintf("search:%s", name)
}
