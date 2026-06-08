// Gateway — central API gateway with circuit breaking, auth, and rate limiting.
// Port 8000. Uses standard net/http (compatible with httputil.ReverseProxy).
//
// Features:
//   - API key validation (RapidAPI, APILayer, direct, anonymous)
//   - Per-IP rate limiting by tier (free 20/min → ultra 1000/min)
//   - Circuit breaking per upstream (auto-failover on repeated errors)
//   - Active health polling every 60s
//   - GET /gateway/status  — live circuit state for all upstreams
//   - POST /gateway/reset/:name  — manual circuit reset (admin)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gateway/internal/auth"
	"gateway/internal/proxy"
)

var (
	reqCount atomic.Int64
	errCount atomic.Int64
)

// ipRateLimiter is a simple per-IP token bucket.
type ipRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens     int
	maxTokens  int
	lastRefill time.Time
	refillRate int // tokens per minute
}

var limiter = &ipRateLimiter{buckets: make(map[string]*bucket)}

func (l *ipRateLimiter) Allow(ip string, maxPerMin int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: maxPerMin, maxTokens: maxPerMin, lastRefill: time.Now(), refillRate: maxPerMin}
		l.buckets[ip] = b
	}

	// Refill tokens based on elapsed time
	elapsed := time.Since(b.lastRefill).Minutes()
	if elapsed > 0 {
		refill := int(elapsed * float64(b.refillRate))
		b.tokens += refill
		if b.tokens > b.maxTokens {
			b.tokens = b.maxTokens
		}
		b.lastRefill = time.Now()
		b.refillRate = maxPerMin
		b.maxTokens = maxPerMin
	}

	if b.tokens <= 0 {
		return false
	}
	b.tokens--
	return true
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.SplitN(xff, ",", 2)[0]
	}
	return r.RemoteAddr
}

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8000")
	adminSecret := envOr("ADMIN_SECRET", "")
	healthPollInterval, _ := time.ParseDuration(envOr("HEALTH_POLL_INTERVAL", "60s"))

	// ── Upstream registry ────────────────────────────────────────────────────
	reg := proxy.NewRegistry(log.Logger)

	type up struct{ prefix, name, url string }
	upstreamDefs := []up{
		{"/v1/insolvency", "insolvency-api", "http://localhost:8080"},
		{"/v1/zvg", "zvg-api", "http://localhost:8081"},
		{"/v1/ted", "ted-api", "http://localhost:8082"},
		{"/v1/trademark", "dpma-api", "http://localhost:8083"},
		{"/v1/sanctions", "sanctions-api", "http://localhost:8084"},
		{"/v1/recalls", "safety-api", "http://localhost:8085"},
		{"/v1/ch", "zefix-api", "http://localhost:8086"},
		{"/v1/bafin", "bafin-api", "http://localhost:8087"},
		{"/v1/lei", "gleif-api", "http://localhost:8089"},
		{"/v1/grants", "cordis-api", "http://localhost:8090"},
		{"/v1/de", "handelsregister-api", "http://localhost:8092"},
		{"/v1/eu-trademark", "euipo-api", "http://localhost:8093"},
		{"/v1/fr", "french-company-api", "http://localhost:8094"},
		{"/v1/uk", "uk-company-api", "http://localhost:8095"},
		{"/v1/research", "research-api", "http://localhost:8096"},
		{"/v1/gdpr", "gdpr-api", "http://localhost:8097"},
		{"/v1/sec", "sec-api", "http://localhost:8098"},
		{"/v1/food", "food-api", "http://localhost:8099"},
		{"/v1/aviation", "aviation-api", "http://localhost:8100"},
		{"/v1/weather", "weather-api", "http://localhost:8101"},
		{"/v1/currency", "currency-api", "http://localhost:8102"},
		{"/v1/drug", "openfda-api", "http://localhost:8103"},
		{"/v1/wikidata", "wikidata-api", "http://localhost:8104"},
		{"/v1/crypto", "crypto-api", "http://localhost:8105"},
		{"/v1/books", "books-api", "http://localhost:8106"},
		{"/v1/ipgeo", "ipgeo-api", "http://localhost:8107"},
		{"/v1/vat", "vat-api", "http://localhost:8108"},
		{"/v1/countries", "countries-api", "http://localhost:8109"},
		{"/v1/chem", "pubchem-api", "http://localhost:8110"},
		{"/v1/nasa", "nasa-api", "http://localhost:8111"},
		{"/v1/pokemon", "pokeapi", "http://localhost:8112"},
		{"/v1/air", "airquality-api", "http://localhost:8113"},
		{"/v1/fx", "exchangerate-api", "http://localhost:8114"},
		{"/v1/bio", "gbif-api", "http://localhost:8115"},
		{"/v1/trivia", "trivia-api", "http://localhost:8116"},
		{"/v1/numbers", "numbers-api", "http://localhost:8117"},
		{"/v1/jokes", "joke-api", "http://localhost:8118"},
		{"/v1/name", "namepredict-api", "http://localhost:8119"},
		{"/v1/worldbank", "worldbank-api", "http://localhost:8120"},
		{"/v1/trials", "clinicaltrials-api", "http://localhost:8121"},
	}
	for _, u := range upstreamDefs {
		if err := reg.Register(u.prefix, u.name, u.url); err != nil {
			log.Fatal().Err(err).Str("name", u.name).Msg("failed to register upstream")
		}
	}

	// Start active health polling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.StartHealthPoller(ctx, healthPollInterval)

	// ── Auth validator ────────────────────────────────────────────────────────
	validator := auth.NewValidator()

	// ── HTTP mux ──────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"service": "api-gateway",
			"port":    port,
		})
	})

	// Gateway status
	mux.HandleFunc("/gateway/status", func(w http.ResponseWriter, r *http.Request) {
		statuses := reg.AllStatuses()
		healthy := 0
		for _, s := range statuses {
			if s.Healthy {
				healthy++
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"total":     len(statuses),
			"healthy":   healthy,
			"unhealthy": len(statuses) - healthy,
			"upstreams": statuses,
		})
	})

	// Circuit reset (admin)
	mux.HandleFunc("/gateway/reset/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST only"})
			return
		}
		if adminSecret != "" && r.Header.Get("X-Admin-Secret") != adminSecret {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/gateway/reset/")
		// Find upstream by name and reset
		found := false
		for _, s := range reg.AllStatuses() {
			if s.Name == name {
				up := reg.Lookup("/v1/" + strings.TrimSuffix(s.Name, "-api"))
				if up != nil {
					up.RecordSuccess()
					found = true
				}
				break
			}
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "upstream not found: " + name})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"reset": true, "upstream": name})
	})

	// Stats
	mux.HandleFunc("/gateway/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"requests": reqCount.Load(),
			"errors":   errCount.Load(),
		})
	})

	// ── Main proxy handler ────────────────────────────────────────────────────
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		path := r.URL.Path

		// Skip gateway internal paths
		if strings.HasPrefix(path, "/gateway/") || path == "/health" {
			http.NotFound(w, r)
			return
		}

		// Auth check for API routes
		if strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/mcp/") {
			result := validator.Validate(r.Header.Get)
			if !result.Valid {
				errCount.Add(1)
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error":   "unauthorized",
					"message": "Valid API key required. See /docs for options.",
				})
				return
			}

			// Tier-based rate limiting
			maxPerMin := auth.RateLimitForTier(result.Tier)
			ip := clientIP(r)
			if !limiter.Allow(ip+":"+result.Tier, maxPerMin) {
				writeJSON(w, http.StatusTooManyRequests, map[string]string{
					"error":   "rate_limit_exceeded",
					"tier":    result.Tier,
					"message": fmt.Sprintf("Limit: %d req/min. Upgrade for higher limits.", maxPerMin),
				})
				return
			}

			// Remove sensitive headers before forwarding
			r.Header.Del("X-Admin-Secret")
		}

		// Find upstream
		up := reg.Lookup(path)
		if up == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{
				"error":   "not_found",
				"message": "No API registered for this path.",
			})
			return
		}

		// Circuit breaker check
		if !up.Allow() {
			status := up.Status()
			errCount.Add(1)
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"error":         "service_unavailable",
				"upstream":      up.Name,
				"circuit_state": status.CircuitState,
				"message":       "This API is temporarily unavailable. Retrying automatically.",
			})
			return
		}

		// Proxy the request and track result
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		up.Proxy.ServeHTTP(rw, r)

		if rw.status >= 500 {
			up.RecordFailure(fmt.Sprintf("upstream returned HTTP %d", rw.status))
			errCount.Add(1)
		} else {
			up.RecordSuccess()
		}
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      mux,
		ReadTimeout:  90 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Info().Str("port", port).Int("upstreams", len(upstreamDefs)).Msg("api-gateway starting")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal().Err(err).Msg("gateway failed")
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
