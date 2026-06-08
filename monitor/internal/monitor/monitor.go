// Package monitor periodically checks health of all APIs and sends alerts
// via Telegram and/or email when an API goes down or recovers.
package monitor

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// APITarget is a single API to monitor.
type APITarget struct {
	Name     string // human-readable, e.g. "sanctions-api"
	HealthURL string // e.g. "http://localhost:8084/health"
	Route    string // nginx path, e.g. "/v1/sanctions/"
}

// Status is the current health state of one API.
type Status struct {
	Name       string    `json:"name"`
	HealthURL  string    `json:"healthUrl"`
	Route      string    `json:"route"`
	Up         bool      `json:"up"`
	LastCheck  time.Time `json:"lastCheck"`
	LastUp     time.Time `json:"lastUp,omitempty"`
	LastDown   time.Time `json:"lastDown,omitempty"`
	ConsecFail int       `json:"consecFails"`   // consecutive failures
	Latency    int       `json:"latencyMs"`
	Error      string    `json:"error,omitempty"`
}

// Monitor polls all APIs and fires alerts on state changes.
type Monitor struct {
	targets  []APITarget
	statuses map[string]*Status
	mu       sync.RWMutex
	alerter  Alerter
	interval time.Duration
	log      zerolog.Logger
	client   *http.Client
	stopCh   chan struct{}
}

// Alerter is anything that can send an alert message.
type Alerter interface {
	Alert(msg string) error
}

// New creates a monitor. interval is how often to check (e.g. 5*time.Minute).
func New(targets []APITarget, alerter Alerter, interval time.Duration, log zerolog.Logger) *Monitor {
	m := &Monitor{
		targets:  targets,
		statuses: make(map[string]*Status, len(targets)),
		alerter:  alerter,
		interval: interval,
		log:      log,
		stopCh:   make(chan struct{}),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
	for _, t := range targets {
		m.statuses[t.Name] = &Status{
			Name:      t.Name,
			HealthURL: t.HealthURL,
			Route:     t.Route,
			Up:        true, // assume up until first check
		}
	}
	return m
}

// Start begins the polling loop (non-blocking).
func (m *Monitor) Start() {
	go m.loop()
}

// Stop stops the polling loop.
func (m *Monitor) Stop() {
	close(m.stopCh)
}

// Snapshot returns a copy of all current statuses.
func (m *Monitor) Snapshot() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.statuses))
	for _, s := range m.statuses {
		out = append(out, *s)
	}
	return out
}

// AllOK returns true if all APIs are currently up.
func (m *Monitor) AllOK() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.statuses {
		if !s.Up {
			return false
		}
	}
	return true
}

func (m *Monitor) loop() {
	// Run immediately on start, then on interval
	m.checkAll()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.checkAll()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Monitor) checkAll() {
	var wg sync.WaitGroup
	for _, t := range m.targets {
		wg.Add(1)
		go func(target APITarget) {
			defer wg.Done()
			m.checkOne(target)
		}(t)
	}
	wg.Wait()
}

func (m *Monitor) checkOne(t APITarget) {
	start := time.Now()
	resp, err := m.client.Get(t.HealthURL)
	latency := int(time.Since(start).Milliseconds())

	m.mu.Lock()
	s := m.statuses[t.Name]
	wasUp := s.Up
	s.LastCheck = time.Now()
	s.Latency = latency

	if err != nil || resp.StatusCode >= 500 {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		} else {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			resp.Body.Close()
			errMsg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))
		}
		s.ConsecFail++
		s.Error = errMsg
		if s.ConsecFail >= 2 {
			s.Up = false
			s.LastDown = time.Now()
		}
	} else {
		resp.Body.Close()
		s.ConsecFail = 0
		s.Error = ""
		s.Up = true
		s.LastUp = time.Now()
	}

	justWentDown := wasUp && !s.Up
	justRecovered := !wasUp && s.Up
	name := t.Name
	consecFails := s.ConsecFail
	errTxt := s.Error
	m.mu.Unlock()

	if justWentDown {
		m.log.Error().Str("api", name).Int("consecFails", consecFails).Str("error", errTxt).Msg("API DOWN")
		msg := fmt.Sprintf("🔴 *API DOWN: %s*\n\nRoute: `%s`\nError: `%s`\nChecked: %s",
			name, t.Route, errTxt, time.Now().Format("15:04:05 UTC"))
		if err := m.alerter.Alert(msg); err != nil {
			m.log.Error().Err(err).Msg("failed to send down alert")
		}
	} else if justRecovered {
		m.log.Info().Str("api", name).Msg("API RECOVERED")
		msg := fmt.Sprintf("✅ *API RECOVERED: %s*\n\nRoute: `%s`\nLatency: %dms\nTime: %s",
			name, t.Route, latency, time.Now().Format("15:04:05 UTC"))
		if err := m.alerter.Alert(msg); err != nil {
			m.log.Error().Err(err).Msg("failed to send recovery alert")
		}
	} else if !s.Up {
		m.log.Warn().Str("api", name).Int("consecFails", consecFails).Msg("API still down")
	} else {
		m.log.Debug().Str("api", name).Int("latencyMs", latency).Msg("health OK")
	}
}
