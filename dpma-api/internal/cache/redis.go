package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrMiss = errors.New("cache miss")

type Cache struct {
	rdb *redis.Client
}

type Options struct {
	Addr     string
	Password string
	DB       int
}

func New(opts Options) (*Cache, error) {
	c := &Cache{
		rdb: redis.NewClient(&redis.Options{
			Addr:     opts.Addr,
			Password: opts.Password,
			DB:       opts.DB,
		}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		c.rdb.Close()
		return nil, err
	}
	return c, nil
}

func (c *Cache) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }
func (c *Cache) Close() error                    { return c.rdb.Close() }

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

func (c *Cache) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, raw, ttl).Err()
}
