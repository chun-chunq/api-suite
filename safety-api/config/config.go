package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port            string
	APIKeys         []string
	AdminSecret     string
	LogLevel        string
	RefreshInterval time.Duration
	RateLimitMax    int
	RateLimitWindow time.Duration
}

func Load() *Config {
	refresh := 24 * time.Hour // EU publishes weekly, daily check is fine
	if s := os.Getenv("REFRESH_INTERVAL"); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			refresh = d
		}
	}
	rlMax := 300
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
				k = k[:idx]
			}
			clean = append(clean, k)
		}
	}
	return &Config{
		Port:            envOr("PORT", "8085"),
		APIKeys:         clean,
		AdminSecret:     os.Getenv("ADMIN_SECRET"),
		LogLevel:        envOr("LOG_LEVEL", "info"),
		RefreshInterval: refresh,
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
