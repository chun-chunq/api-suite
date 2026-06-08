// Package circuitbreaker implements a thread-safe circuit breaker
// that can wrap any outbound HTTP call.
//
// States:
//   Closed   → normal operation, requests flow through
//   Open     → breaker tripped, requests fail fast with ErrOpen
//   HalfOpen → testing if backend recovered (one probe at a time)
//
// Transitions:
//   Closed  → Open      : after ConsecutiveFailures errors in a row
//   Open    → HalfOpen  : after Timeout has elapsed
//   HalfOpen→ Closed    : on a successful probe request
//   HalfOpen→ Open      : on a failed probe request (resets timeout)
package circuitbreaker

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrOpen is returned when the circuit is open and requests are blocked.
var ErrOpen = errors.New("circuit_open: upstream unavailable, retrying soon")

// State is the circuit breaker state.
type State int

const (
	StateClosed   State = iota // normal
	StateOpen                  // tripped
	StateHalfOpen              // testing recovery
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker settings.
type Config struct {
	// Number of consecutive failures before the breaker opens.
	ConsecutiveFailures int

	// How long to stay open before probing again.
	Timeout time.Duration

	// Optional callback when state changes.
	OnStateChange func(name string, from, to State)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		ConsecutiveFailures: 5,
		Timeout:             30 * time.Second,
	}
}

// Breaker is a circuit breaker instance.
type Breaker struct {
	name   string
	cfg    Config
	mu     sync.Mutex
	state  State
	fails  int
	openAt time.Time
}

// New creates a new circuit breaker.
func New(name string, cfg Config) *Breaker {
	if cfg.ConsecutiveFailures == 0 {
		cfg.ConsecutiveFailures = DefaultConfig().ConsecutiveFailures
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultConfig().Timeout
	}
	return &Breaker{name: name, cfg: cfg, state: StateClosed}
}

// Allow reports whether a request is allowed through.
// Call RecordSuccess or RecordFailure after the request completes.
func (b *Breaker) Allow() (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		return true, nil

	case StateOpen:
		if time.Since(b.openAt) < b.cfg.Timeout {
			return false, fmt.Errorf("%w (will retry after %s)",
				ErrOpen, b.openAt.Add(b.cfg.Timeout).Format("15:04:05"))
		}
		// Transition to half-open: let one probe through
		b.setState(StateHalfOpen)
		return true, nil

	case StateHalfOpen:
		// Only one probe at a time
		return false, fmt.Errorf("%w (probe in progress)", ErrOpen)
	}

	return true, nil
}

// RecordSuccess records a successful call.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fails = 0
	if b.state != StateClosed {
		b.setState(StateClosed)
	}
}

// RecordFailure records a failed call.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fails++
	if b.state == StateHalfOpen || b.fails >= b.cfg.ConsecutiveFailures {
		b.setState(StateOpen)
		b.openAt = time.Now()
	}
}

// State returns the current state (safe for concurrent reads).
func (b *Breaker) GetState() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Stats returns a snapshot of the breaker state.
func (b *Breaker) Stats() map[string]interface{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := map[string]interface{}{
		"name":   b.name,
		"state":  b.state.String(),
		"fails":  b.fails,
		"threshold": b.cfg.ConsecutiveFailures,
	}
	if b.state == StateOpen {
		remaining := b.cfg.Timeout - time.Since(b.openAt)
		if remaining < 0 {
			remaining = 0
		}
		m["retry_in_seconds"] = int(remaining.Seconds())
	}
	return m
}

func (b *Breaker) setState(newState State) {
	if b.state == newState {
		return
	}
	old := b.state
	b.state = newState
	if b.cfg.OnStateChange != nil {
		go b.cfg.OnStateChange(b.name, old, newState)
	}
}

// ── Registry ─────────────────────────────────────────────────────────────────

// Registry holds named circuit breakers.
type Registry struct {
	mu       sync.RWMutex
	breakers map[string]*Breaker
	cfg      Config
}

// NewRegistry creates a registry where all breakers share the same config.
func NewRegistry(cfg Config) *Registry {
	return &Registry{
		breakers: make(map[string]*Breaker),
		cfg:      cfg,
	}
}

// Get returns (or creates) a named breaker.
func (r *Registry) Get(name string) *Breaker {
	r.mu.RLock()
	if b, ok := r.breakers[name]; ok {
		r.mu.RUnlock()
		return b
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// Double-check after acquiring write lock
	if b, ok := r.breakers[name]; ok {
		return b
	}
	b := New(name, r.cfg)
	r.breakers[name] = b
	return b
}

// AllStats returns stats for all registered breakers.
func (r *Registry) AllStats() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]map[string]interface{}, 0, len(r.breakers))
	for _, b := range r.breakers {
		result = append(result, b.Stats())
	}
	return result
}

// Do executes fn through the named circuit breaker.
// fn should return an error on failure, nil on success.
func (r *Registry) Do(name string, fn func() error) error {
	b := r.Get(name)
	ok, err := b.Allow()
	if !ok {
		return err
	}
	fnErr := fn()
	if fnErr != nil {
		b.RecordFailure()
	} else {
		b.RecordSuccess()
	}
	return fnErr
}
