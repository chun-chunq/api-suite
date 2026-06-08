package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMiss is returned when a key is not present in the cache.
var ErrMiss = errors.New("cache miss")

// Cache is a thin JSON-encoding wrapper over a Redis client.
type Cache struct {
	rdb *redis.Client
}

// New constructs a Cache. It does not block on connectivity; callers may use
// Ping to verify the backend is reachable.
func New(addr, password string, db int) *Cache {
	return &Cache{
		rdb: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

// Ping verifies connectivity to Redis.
func (c *Cache) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close releases the underlying connection pool.
func (c *Cache) Close() error {
	return c.rdb.Close()
}

// GetJSON loads and unmarshals a cached value into dst. Returns ErrMiss when the
// key is absent.
func (c *Cache) GetJSON(ctx context.Context, key string, dst any) error {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrMiss
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

// SetJSON marshals and stores a value with the given TTL.
func (c *Cache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, raw, ttl).Err()
}

// Client exposes the raw client for components (e.g. asynq) that need it.
func (c *Cache) Client() *redis.Client { return c.rdb }
