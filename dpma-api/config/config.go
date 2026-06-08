package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port              string
	RedisAddr         string
	RedisPassword     string
	RedisDB           int
	APIKeys           []string
	RateLimitMax      int
	RateLimitWindow   time.Duration
	CacheTTL          time.Duration
	LogLevel          string
	Environment       string
	ChromeBin         string
	WorkerURLs        []string
	WorkerSecret      string
	AdminSecret       string // for /admin/workers dynamic management
	MaxBrowsers       int    // concurrent Chrome instances (idle optimization)
	MaxQueueDepth     int    // max requests waiting for a browser slot before 429
	BandwidthLimitGB  int64  // soft monthly limit in GB (0 = disabled)
}

func Load() *Config {
	return &Config{
		Port:             getenv("PORT", "8083"),
		RedisAddr:        getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:    getenv("REDIS_PASSWORD", ""),
		RedisDB:          getenvInt("REDIS_DB", 0),
		APIKeys:          parseAPIKeys(getenv("API_KEYS", "")),
		RateLimitMax:     getenvInt("RATE_LIMIT_MAX", 30), // DPMA may block at high rates
		RateLimitWindow:  getenvDuration("RATE_LIMIT_WINDOW", time.Hour),
		CacheTTL:         getenvDuration("CACHE_TTL", 24*time.Hour), // trademark data changes rarely
		LogLevel:         getenv("LOG_LEVEL", "info"),
		Environment:      getenv("ENVIRONMENT", "development"),
		ChromeBin:        getenv("CHROME_BIN", ""),
		WorkerURLs:       splitNonEmpty(getenv("WORKER_URLS", "")),
		WorkerSecret:     getenv("WORKER_SECRET", ""),
		AdminSecret:      getenv("ADMIN_SECRET", ""),
		MaxBrowsers:      getenvInt("MAX_BROWSERS", 2),
		MaxQueueDepth:    getenvInt("MAX_QUEUE_DEPTH", 20),
		BandwidthLimitGB: getenvInt64("BANDWIDTH_LIMIT_GB", 15), // 15TB safety margin for Hetzner 20TB plans
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

func getenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func splitNonEmpty(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseAPIKeys(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			if idx := strings.Index(p, ":"); idx != -1 {
				p = p[:idx]
			}
			out = append(out, p)
		}
	}
	return out
}
