// Package proxy manages the upstream API registry with circuit breaking,
// health tracking, and automatic failover.
package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// UpstreamStatus describes an upstream service's current health.
type UpstreamStatus struct {
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	Healthy       bool      `json:"healthy"`
	ConsecFails   int       `json:"consecutive_failures"`
	CircuitState  string    `json:"circuit_state"` // closed / open / half-open
	LastCheck     time.Time `json:"last_check"`
	LastError     string    `json:"last_error,omitempty"`
	RetryAfter    time.Time `json:"retry_after,omitempty"`
}

// Upstream is one backend API service.
type Upstream struct {
	Name    string
	BaseURL *url.URL
	Proxy   *httputil.ReverseProxy

	mu           sync.Mutex
	healthy      bool
	consecFails  int
	circuitOpen  bool
	openAt       time.Time
	lastError    string
	lastCheck    time.Time
}

const (
	circuitOpenThreshold = 5             // failures before opening
	circuitTimeout       = 30 * time.Second // time before probing
)

// Allow returns true if the upstream can receive requests.
func (u *Upstream) Allow() bool {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.circuitOpen {
		return true
	}
	// Check if we can probe again
	if time.Since(u.openAt) >= circuitTimeout {
		return true // half-open: allow one probe
	}
	return false
}

// RecordSuccess marks a successful request.
func (u *Upstream) RecordSuccess() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.consecFails = 0
	u.circuitOpen = false
	u.healthy = true
	u.lastCheck = time.Now()
	u.lastError = ""
}

// RecordFailure marks a failed request.
func (u *Upstream) RecordFailure(err string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.consecFails++
	u.lastError = err
	u.lastCheck = time.Now()
	u.healthy = false
	if u.consecFails >= circuitOpenThreshold {
		u.circuitOpen = true
		u.openAt = time.Now()
	}
}

// Status returns a safe snapshot.
func (u *Upstream) Status() UpstreamStatus {
	u.mu.Lock()
	defer u.mu.Unlock()
	state := "closed"
	var retryAfter time.Time
	if u.circuitOpen {
		if time.Since(u.openAt) >= circuitTimeout {
			state = "half-open"
		} else {
			state = "open"
			retryAfter = u.openAt.Add(circuitTimeout)
		}
	}
	return UpstreamStatus{
		Name:         u.Name,
		URL:          u.BaseURL.String(),
		Healthy:      u.healthy,
		ConsecFails:  u.consecFails,
		CircuitState: state,
		LastCheck:    u.lastCheck,
		LastError:    u.lastError,
		RetryAfter:   retryAfter,
	}
}

// ── Registry ─────────────────────────────────────────────────────────────────

// Registry maps route prefixes to upstream instances.
type Registry struct {
	mu        sync.RWMutex
	upstreams map[string]*Upstream // key = route prefix (e.g. "/v1/vat")
	log       zerolog.Logger
}

// NewRegistry creates an empty registry.
func NewRegistry(log zerolog.Logger) *Registry {
	return &Registry{
		upstreams: make(map[string]*Upstream),
		log:       log,
	}
}

// Register adds a named upstream for a given route prefix.
func (r *Registry) Register(prefix, name, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid upstream URL %q: %w", rawURL, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, proxyErr error) {
		// Errors logged elsewhere; return 502
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{
			"error":   "upstream_error",
			"message": "upstream temporarily unavailable",
		})
	}

	up := &Upstream{
		Name:    name,
		BaseURL: u,
		Proxy:   proxy,
		healthy: true, // optimistic start
	}

	r.mu.Lock()
	r.upstreams[prefix] = up
	r.mu.Unlock()
	return nil
}

// Lookup finds the upstream for a request path.
// Returns the longest matching prefix.
func (r *Registry) Lookup(path string) *Upstream {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *Upstream
	bestLen := 0
	for prefix, up := range r.upstreams {
		if len(prefix) > bestLen && len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			bestLen = len(prefix)
			best = up
		}
	}
	return best
}

// AllStatuses returns a snapshot of all upstream statuses.
func (r *Registry) AllStatuses() []UpstreamStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]UpstreamStatus, 0, len(r.upstreams))
	for _, up := range r.upstreams {
		result = append(result, up.Status())
	}
	return result
}

// StartHealthPoller starts a background goroutine that polls /health
// on every registered upstream every interval, updating circuit state.
func (r *Registry) StartHealthPoller(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.pollAll(ctx)
			}
		}
	}()
}

func (r *Registry) pollAll(ctx context.Context) {
	r.mu.RLock()
	ups := make([]*Upstream, 0, len(r.upstreams))
	for _, up := range r.upstreams {
		ups = append(ups, up)
	}
	r.mu.RUnlock()

	var wg sync.WaitGroup
	for _, up := range ups {
		wg.Add(1)
		go func(u *Upstream) {
			defer wg.Done()
			r.pollOne(ctx, u)
		}(up)
	}
	wg.Wait()
}

func (r *Registry) pollOne(ctx context.Context, u *Upstream) {
	healthURL := u.BaseURL.String() + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		u.RecordFailure(err.Error())
		r.log.Warn().Str("upstream", u.Name).Err(err).Msg("health check failed")
		return
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		wasOpen := u.Status().CircuitState != "closed"
		u.RecordSuccess()
		if wasOpen {
			r.log.Info().Str("upstream", u.Name).Msg("circuit closed — upstream recovered")
		}
	} else {
		u.RecordFailure(fmt.Sprintf("HTTP %d", resp.StatusCode))
		r.log.Warn().Str("upstream", u.Name).Int("status", resp.StatusCode).Msg("health check returned non-200")
	}
}
