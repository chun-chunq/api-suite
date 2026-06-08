package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration sourced from environment variables.
type Config struct {
	Env  string
	Port string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// CacheTTL is how long company data is cached. Default 24h.
	CacheTTL time.Duration

	// APIKeys is the set of valid keys. In production this would be backed by a
	// datastore; for now it is seeded from a comma-separated env var.
	APIKeys map[string]bool

	// RateLimitPerMinute is the per-API-key request budget per minute.
	RateLimitPerMinute int

	// ScrapeTimeout bounds a single scrape operation.
	ScrapeTimeout time.Duration

	// BrowserBin optionally points rod at a system Chromium binary.
	BrowserBin string

	// WorkerConcurrency controls how many asynq jobs run in parallel.
	WorkerConcurrency int
}

// Load reads configuration from the environment, applying sane defaults.
func Load() (*Config, error) {
	cfg := &Config{
		Env:                getEnv("APP_ENV", "development"),
		Port:               getEnv("PORT", "8080"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		BrowserBin:         getEnv("BROWSER_BIN", ""),
		RedisDB:            getEnvInt("REDIS_DB", 0),
		CacheTTL:           getEnvDuration("CACHE_TTL", 24*time.Hour),
		RateLimitPerMinute: getEnvInt("RATE_LIMIT_PER_MINUTE", 60),
		ScrapeTimeout:      getEnvDuration("SCRAPE_TIMEOUT", 45*time.Second),
		WorkerConcurrency:  getEnvInt("WORKER_CONCURRENCY", 5),
		APIKeys:            parseAPIKeys(getEnv("API_KEYS", "dev-key-123")),
	}

	if len(cfg.APIKeys) == 0 {
		return nil, fmt.Errorf("config: no API keys configured (set API_KEYS)")
	}

	return cfg, nil
}

func parseAPIKeys(raw string) map[string]bool {
	keys := make(map[string]bool)
	for _, k := range strings.Split(raw, ",") {
		k = strings.TrimSpace(k)
		if k != "" {
			if idx := strings.Index(k, ":"); idx != -1 {
				k = k[:idx]
			}
			keys[k] = true
		}
	}
	return keys
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
