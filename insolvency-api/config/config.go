package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration, populated from environment variables
// with sensible defaults so the service runs out of the box for local testing.
type Config struct {
	Port           string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	APIKeys        []string      // valid API keys for the apikey middleware
	RateLimitMax   int           // requests allowed per window
	RateLimitWindow time.Duration
	SearchCacheTTL time.Duration
	RecordCacheTTL time.Duration
	LogLevel       string
	Environment    string
	WorkerURLs     []string
	ChromeBin      string
	WorkerSecret   string
	AdminSecret    string
	MaxBrowsers    int
	MaxQueueDepth  int
}

// Load reads configuration from the environment.
func Load() *Config {
	return &Config{
		Port:            getenv("PORT", "8080"),
		RedisAddr:       getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   getenv("REDIS_PASSWORD", ""),
		RedisDB:         getenvInt("REDIS_DB", 0),
		APIKeys:         parseAPIKeys(getenv("API_KEYS", "")),
		RateLimitMax:    getenvInt("RATE_LIMIT_MAX", 60),
		RateLimitWindow: getenvDuration("RATE_LIMIT_WINDOW", time.Hour), // 60/h — handelsregister.de ToS limit
		SearchCacheTTL:  getenvDuration("SEARCH_CACHE_TTL", time.Hour),
		RecordCacheTTL:  getenvDuration("RECORD_CACHE_TTL", 24*time.Hour),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		Environment:     getenv("ENVIRONMENT", "development"),
		WorkerURLs:    splitNonEmpty(getenv("WORKER_URLS", "")),
		ChromeBin:     getenv("CHROME_BIN", ""),
		WorkerSecret:  getenv("WORKER_SECRET", ""),
		AdminSecret:   getenv("ADMIN_SECRET", ""),
		MaxBrowsers:   getenvInt("MAX_BROWSERS", 2),
		MaxQueueDepth: getenvInt("MAX_QUEUE_DEPTH", 20),
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
