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
	APIKeys         []string
	AdminSecret     string
	LogLevel        string
	RefreshInterval time.Duration // how often to re-download the sanctions list
	RateLimitMax    int
	RateLimitWindow time.Duration
}

func Load() *Config {
	refresh := 6 * time.Hour
	if s := os.Getenv("REFRESH_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			refresh = d
		}
	}
	rlMax := 120
	if s := os.Getenv("RATE_LIMIT_MAX"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			rlMax = n
		}
	}
	rlWindow := time.Hour
	if s := os.Getenv("RATE_LIMIT_WINDOW"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			rlWindow = d
		}
	}

	keys := strings.Split(os.Getenv("API_KEYS"), ",")
	clean := keys[:0]
	for _, k := range keys {
		if k = strings.TrimSpace(k); k != "" {
			if idx := strings.Index(k, ":"); idx != -1 {
				k = k[:idx] // strip :tier suffix (key:ultra → key)
			}
			clean = append(clean, k)
		}
	}

	return &Config{
		Port:            envOr("PORT", "8084"),
		RedisAddr:       envOr("REDIS_ADDR", "localhost:6379"),
		APIKeys:         clean,
		AdminSecret:     os.Getenv("ADMIN_SECRET"),
		LogLevel:        envOr("LOG_LEVEL", "info"),
		RefreshInterval: refresh,
		RateLimitMax:    rlMax,
		RateLimitWindow: rlWindow,
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
