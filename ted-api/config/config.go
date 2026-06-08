package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	APIKeys         []string
	RateLimitMax    int
	RateLimitWindow time.Duration
	CacheTTL        time.Duration
	LogLevel        string
	Environment     string
}

func Load() *Config {
	return &Config{
		Port:            getenv("PORT", "8080"),
		RedisAddr:       getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:   getenv("REDIS_PASSWORD", ""),
		RedisDB:         getenvInt("REDIS_DB", 0),
		APIKeys:         parseAPIKeys(getenv("API_KEYS", "")),
		RateLimitMax:    getenvInt("RATE_LIMIT_MAX", 60),
		RateLimitWindow: getenvDuration("RATE_LIMIT_WINDOW", time.Hour),
		CacheTTL:        getenvDuration("CACHE_TTL", 4*time.Hour),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		Environment:     getenv("ENVIRONMENT", "development"),
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func getenvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}
func getenvDuration(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			return dur
		}
	}
	return d
}
func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseAPIKeys(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			if idx := strings.Index(p, ":"); idx != -1 {
				p = p[:idx]
			}
			out = append(out, p)
		}
	}
	return out
}
