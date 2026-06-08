// Package pool implements a dynamic scrape-worker pool with hot-add/remove,
// health checking, and automatic cooldown on repeated failures.
package pool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	cooldownDuration = 5 * time.Minute
	workerTimeout    = 90 * time.Second
	maxFailures      = 3
	failureWindow    = 10 * time.Minute
	healthInterval   = 30 * time.Second
)

var ErrNoWorkers = errors.New("all scrape workers are in cooldown — retry later")

type workerState struct {
	url       string
	mu        sync.Mutex
	failures  []time.Time
	coolUntil time.Time
	healthy   bool
}

func (w *workerState) isAvailable() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Now().Before(w.coolUntil) {
		return false
	}
	return w.healthy
}

func (w *workerState) recordFailure() {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-failureWindow)
	kept := w.failures[:0]
	for _, t := range w.failures {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	w.failures = append(kept, now)
	if len(w.failures) >= maxFailures {
		w.coolUntil = now.Add(cooldownDuration)
		w.failures = nil
	}
}

func (w *workerState) recordSuccess() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failures = nil
	w.healthy = true
}

// Pool is a dynamically managed set of remote scrape workers.
type Pool struct {
	mu      sync.RWMutex
	workers []*workerState
	idx     int
	log     zerolog.Logger
	http    *http.Client
}

// New creates a Pool from an initial list of worker URLs.
// URLs can be empty — use AddWorker to add dynamically.
func New(urls []string, log zerolog.Logger) *Pool {
	p := &Pool{
		log:  log,
		http: &http.Client{Timeout: workerTimeout + 5*time.Second},
	}
	for _, u := range urls {
		p.workers = append(p.workers, &workerState{url: u, healthy: true})
	}
	go p.runHealthChecks()
	return p
}

// HasWorkers returns true if at least one worker is currently available.
func (p *Pool) HasWorkers() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, w := range p.workers {
		if w.isAvailable() {
			return true
		}
	}
	return false
}

// AddWorker adds a new worker URL at runtime. Idempotent.
func (p *Pool) AddWorker(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, w := range p.workers {
		if w.url == url {
			return // already present
		}
	}
	p.workers = append(p.workers, &workerState{url: url, healthy: true})
	p.log.Info().Str("url", url).Msg("worker added to pool")
}

// RemoveWorker removes a worker URL from the pool.
func (p *Pool) RemoveWorker(url string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, w := range p.workers {
		if w.url == url {
			p.workers = append(p.workers[:i], p.workers[i+1:]...)
			p.log.Info().Str("url", url).Msg("worker removed from pool")
			return true
		}
	}
	return false
}

// Dispatch sends a scrape request to the next available worker (round-robin).
func (p *Pool) Dispatch(ctx context.Context, path string, payload, result any) error {
	p.mu.RLock()
	workers := make([]*workerState, len(p.workers))
	copy(workers, p.workers)
	p.mu.RUnlock()

	if len(workers) == 0 {
		return ErrNoWorkers
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("payload marshal: %w", err)
	}

	// Round-robin over available workers
	p.mu.Lock()
	start := p.idx
	p.idx = (p.idx + 1) % len(workers)
	p.mu.Unlock()

	for i := range workers {
		w := workers[(start+i)%len(workers)]
		if !w.isAvailable() {
			continue
		}
		if err := p.call(ctx, w, path, body, result); err == nil {
			w.recordSuccess()
			return nil
		} else {
			p.log.Warn().Err(err).Str("worker", w.url).Msg("worker call failed")
			w.recordFailure()
		}
	}
	return ErrNoWorkers
}

// Status returns current health status of all workers.
func (p *Pool) Status() []map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]map[string]any, len(p.workers))
	for i, w := range p.workers {
		w.mu.Lock()
		cool := ""
		if time.Now().Before(w.coolUntil) {
			cool = w.coolUntil.Format(time.RFC3339)
		}
		out[i] = map[string]any{
			"url":          w.url,
			"healthy":      w.healthy,
			"available":    w.isAvailable(),
			"cooldownUntil": cool,
		}
		w.mu.Unlock()
	}
	return out
}

func (p *Pool) call(ctx context.Context, w *workerState, path string, body []byte, result any) error {
	ctx, cancel := context.WithTimeout(ctx, workerTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return errors.New("worker returned 503")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("worker returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, result)
}

// runHealthChecks periodically pings each worker's /health endpoint.
func (p *Pool) runHealthChecks() {
	ticker := time.NewTicker(healthInterval)
	defer ticker.Stop()
	for range ticker.C {
		p.mu.RLock()
		workers := make([]*workerState, len(p.workers))
		copy(workers, p.workers)
		p.mu.RUnlock()

		for _, w := range workers {
			go func(w *workerState) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, w.url+"/health", nil)
				resp, err := p.http.Do(req)
				w.mu.Lock()
				defer w.mu.Unlock()
				if err == nil && resp.StatusCode < 300 {
					resp.Body.Close()
					if !w.healthy {
						w.healthy = true
						p.log.Info().Str("worker", w.url).Msg("worker came back online")
					}
				} else {
					if w.healthy {
						p.log.Warn().Str("worker", w.url).Msg("worker health check failed")
					}
					w.healthy = false
				}
			}(w)
		}
	}
}
