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
	ChromeBin       string
	MaxBrowsers     int
	MaxQueueDepth   int
	CacheTTL        time.Duration
	RateLimitMax    int
	RateLimitWindow time.Duration
}

func Load() *Config {
	maxBrowsers := 2
	if s := os.Getenv("MAX_BROWSERS"); s != "" {
		if n, _ := strconv.Atoi(s); n > 0 {
			maxBrowsers = n
		}
	}
	maxDepth := 20
	if s := os.Getenv("MAX_QUEUE_DEPTH"); s != "" {
		if n, _ := strconv.Atoi(s); n > 0 {
			maxDepth = n
		}
	}
	cacheTTL := 24 * time.Hour // BaFin license data changes rarely
	if s := os.Getenv("CACHE_TTL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			cacheTTL = d
		}
	}
	rlMax := 30
	if s := os.Getenv("RATE_LIMIT_MAX"); s != "" {
		if n, _ := strconv.Atoi(s); n > 0 {
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
				k = k[:idx]
			}
			clean = append(clean, k)
		}
	}
	return &Config{
		Port:            envOr("PORT", "8087"),
		RedisAddr:       envOr("REDIS_ADDR", "localhost:6379"),
		APIKeys:         clean,
		AdminSecret:     os.Getenv("ADMIN_SECRET"),
		LogLevel:        envOr("LOG_LEVEL", "info"),
		ChromeBin:       envOr("CHROME_BIN", "/usr/bin/chromium"),
		MaxBrowsers:     maxBrowsers,
		MaxQueueDepth:   maxDepth,
		CacheTTL:        cacheTTL,
		RateLimitMax:    rlMax,
		RateLimitWindow: rlWindow,
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
