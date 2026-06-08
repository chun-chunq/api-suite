// Package analytics provides zero-allocation request tracking.
//
// Design goals:
//   - Atomic counters on the hot path (no mutex, no allocation per request)
//   - Ring buffer for recent requests (last 500, lock-free writes)
//   - Per-endpoint AND per-API-key usage counters
//   - Periodic structured log summary (every 5 min, async goroutine)
//   - JSON snapshot at /admin/analytics
package analytics

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

const recentSize = 500 // ring buffer capacity

// Record is a single request record stored in the ring buffer.
type Record struct {
	Time        int64  // UnixMilli
	Endpoint    string
	Method      string
	StatusCode  int32
	LatencyMs   int64
	QueueWaitMs int64
	APIKey      string // raw key — hashed internally for per-key tracking
	Cached      bool
	Scrape      bool // was a scrape triggered
	Error       bool
}

// KeyHash returns a stable uint32 FNV-1a hash of the API key.
func KeyHash(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}

// Analytics tracks request metrics with minimal overhead.
type Analytics struct {
	startTime time.Time

	// hot-path counters — all atomic
	totalRequests  atomic.Int64
	totalErrors    atomic.Int64
	cacheHits      atomic.Int64
	cacheMisses    atomic.Int64
	scrapeSuccess  atomic.Int64
	scrapeFailure  atomic.Int64
	queueRejected  atomic.Int64
	queueTimedOut  atomic.Int64
	totalLatencyMs atomic.Int64

	// ring buffer for recent requests (lock-free writes via atomic index)
	recent    [recentSize]Record
	recentIdx atomic.Int64

	// per-endpoint counters (RW mutex, lazy init)
	epMu      sync.RWMutex
	endpoints map[string]*endpointStats

	// per-API-key counters — tracks how often each customer uses the API
	keyMu  sync.RWMutex
	perKey map[uint32]*keyStats

	log zerolog.Logger
}

type endpointStats struct {
	requests  atomic.Int64
	errors    atomic.Int64
	latencyMs atomic.Int64 // sum
}

type keyStats struct {
	hint     string       // last 6 chars of key for identification
	requests atomic.Int64 // total calls
	lastSeen atomic.Int64 // unix seconds
	// per-endpoint call counts for this key
	epMu sync.Mutex
	eps  map[string]*atomic.Int64
}

// New creates an Analytics instance and starts the periodic log goroutine.
func New(log zerolog.Logger) *Analytics {
	a := &Analytics{
		startTime: time.Now(),
		endpoints: make(map[string]*endpointStats),
		perKey:    make(map[uint32]*keyStats),
		log:       log,
	}
	go a.periodicLog()
	return a
}

// Record records a completed request. Non-blocking; safe to call from any goroutine.
func (a *Analytics) Record(r Record) {
	// global atomic counters
	a.totalRequests.Add(1)
	a.totalLatencyMs.Add(r.LatencyMs)
	if r.Error {
		a.totalErrors.Add(1)
	}
	if r.Cached {
		a.cacheHits.Add(1)
	} else if r.Scrape {
		a.cacheMisses.Add(1)
	}
	if r.Scrape {
		if r.Error {
			a.scrapeFailure.Add(1)
		} else {
			a.scrapeSuccess.Add(1)
		}
	}

	// ring buffer — atomic slot reservation (no mutex)
	slot := int(a.recentIdx.Add(1)-1) % recentSize
	a.recent[slot] = r

	// per-endpoint stats (lazy init)
	if r.Endpoint != "" {
		a.epMu.RLock()
		ep := a.endpoints[r.Endpoint]
		a.epMu.RUnlock()
		if ep == nil {
			a.epMu.Lock()
			if a.endpoints[r.Endpoint] == nil {
				a.endpoints[r.Endpoint] = &endpointStats{}
			}
			ep = a.endpoints[r.Endpoint]
			a.epMu.Unlock()
		}
		ep.requests.Add(1)
		ep.latencyMs.Add(r.LatencyMs)
		if r.Error {
			ep.errors.Add(1)
		}
	}

	// per-API-key tracking — who calls how often
	if r.APIKey != "" {
		hash := KeyHash(r.APIKey)

		a.keyMu.RLock()
		ks := a.perKey[hash]
		a.keyMu.RUnlock()

		if ks == nil {
			hint := r.APIKey
			if len(hint) > 6 {
				hint = "…" + hint[len(hint)-6:]
			}
			a.keyMu.Lock()
			if a.perKey[hash] == nil {
				a.perKey[hash] = &keyStats{
					hint: hint,
					eps:  make(map[string]*atomic.Int64),
				}
			}
			ks = a.perKey[hash]
			a.keyMu.Unlock()
		}

		ks.requests.Add(1)
		ks.lastSeen.Store(time.Now().Unix())

		if r.Endpoint != "" {
			ks.epMu.Lock()
			epCtr := ks.eps[r.Endpoint]
			if epCtr == nil {
				epCtr = &atomic.Int64{}
				ks.eps[r.Endpoint] = epCtr
			}
			ks.epMu.Unlock()
			epCtr.Add(1)
		}
	}
}

// RecordQueueRejected records a queue-full rejection.
func (a *Analytics) RecordQueueRejected() { a.queueRejected.Add(1) }

// RecordQueueTimeout records a request that timed out waiting in queue.
func (a *Analytics) RecordQueueTimeout() { a.queueTimedOut.Add(1) }

// Snapshot returns a JSON-serialisable analytics summary.
func (a *Analytics) Snapshot() map[string]any {
	total := a.totalRequests.Load()
	errors := a.totalErrors.Load()
	hits := a.cacheHits.Load()
	misses := a.cacheMisses.Load()
	scrapeOK := a.scrapeSuccess.Load()
	scrapeFail := a.scrapeFailure.Load()

	var avgLatMs float64
	if total > 0 {
		avgLatMs = float64(a.totalLatencyMs.Load()) / float64(total)
	}
	var hitRate float64
	if hm := hits + misses; hm > 0 {
		hitRate = float64(hits) * 100 / float64(hm)
	}
	var errRate float64
	if total > 0 {
		errRate = float64(errors) * 100 / float64(total)
	}

	// per-endpoint snapshot
	a.epMu.RLock()
	epSnap := make(map[string]any, len(a.endpoints))
	for path, ep := range a.endpoints {
		reqs := ep.requests.Load()
		var avg float64
		if reqs > 0 {
			avg = float64(ep.latencyMs.Load()) / float64(reqs)
		}
		epSnap[path] = map[string]any{
			"requests":     reqs,
			"errors":       ep.errors.Load(),
			"avgLatencyMs": avg,
		}
	}
	a.epMu.RUnlock()

	// per-API-key snapshot — sorted by request count descending
	type keyEntry struct {
		hint     string
		requests int64
		lastSeen int64
		eps      map[string]int64
	}
	a.keyMu.RLock()
	entries := make([]keyEntry, 0, len(a.perKey))
	for _, ks := range a.perKey {
		ks.epMu.Lock()
		epCopy := make(map[string]int64, len(ks.eps))
		for ep, ctr := range ks.eps {
			epCopy[ep] = ctr.Load()
		}
		ks.epMu.Unlock()
		entries = append(entries, keyEntry{
			hint:     ks.hint,
			requests: ks.requests.Load(),
			lastSeen: ks.lastSeen.Load(),
			eps:      epCopy,
		})
	}
	a.keyMu.RUnlock()

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].requests > entries[j].requests
	})
	keySnap := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		m := map[string]any{
			"key":         e.hint,    // last 6 chars — identifies key without exposing it
			"totalCalls":  e.requests,
			"perEndpoint": e.eps,
		}
		if e.lastSeen > 0 {
			m["lastSeen"] = time.Unix(e.lastSeen, 0).UTC().Format(time.RFC3339)
		}
		keySnap = append(keySnap, m)
	}

	// recent ring-buffer snapshot (last 20 records)
	idx := int(a.recentIdx.Load())
	count := recentSize
	if idx < recentSize {
		count = idx
	}
	recent := make([]map[string]any, 0, min(20, count))
	start := idx - min(20, count)
	if start < 0 {
		start = 0
	}
	for i := start; i < idx && i < recentSize+start; i++ {
		r := a.recent[i%recentSize]
		if r.Time == 0 {
			continue
		}
		recent = append(recent, map[string]any{
			"time":        time.UnixMilli(r.Time).Format(time.RFC3339),
			"endpoint":    r.Endpoint,
			"status":      r.StatusCode,
			"latencyMs":   r.LatencyMs,
			"queueWaitMs": r.QueueWaitMs,
			"cached":      r.Cached,
			"error":       r.Error,
		})
	}

	return map[string]any{
		"uptime": time.Since(a.startTime).Round(time.Second).String(),
		"requests": map[string]any{
			"total":        total,
			"errors":       errors,
			"errorRate":    fmt.Sprintf("%.2f%%", errRate),
			"avgLatencyMs": avgLatMs,
		},
		"cache": map[string]any{
			"hits":    hits,
			"misses":  misses,
			"hitRate": fmt.Sprintf("%.2f%%", hitRate),
		},
		"scraper": map[string]any{
			"success":       scrapeOK,
			"failure":       scrapeFail,
			"queueRejected": a.queueRejected.Load(),
			"queueTimedOut": a.queueTimedOut.Load(),
		},
		"perEndpoint": epSnap,
		"perAPIKey":   keySnap, // ← who calls how often: sorted by totalCalls desc
		"recent":      recent,
	}
}

// periodicLog logs a compact analytics summary every 5 minutes.
func (a *Analytics) periodicLog() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		total := a.totalRequests.Load()
		if total == 0 {
			continue
		}
		errors := a.totalErrors.Load()
		hits := a.cacheHits.Load()
		misses := a.cacheMisses.Load()
		var avgLat float64
		if total > 0 {
			avgLat = float64(a.totalLatencyMs.Load()) / float64(total)
		}
		var hitRate float64
		if h := hits + misses; h > 0 {
			hitRate = float64(hits) * 100 / float64(h)
		}
		a.keyMu.RLock()
		uniqueKeys := len(a.perKey)
		a.keyMu.RUnlock()
		a.log.Info().
			Int64("totalReqs", total).
			Int64("errors", errors).
			Float64("errPct", float64(errors)*100/float64(total)).
			Int64("cacheHits", hits).
			Float64("cacheHitPct", hitRate).
			Float64("avgLatMs", avgLat).
			Int64("scrapeOK", a.scrapeSuccess.Load()).
			Int64("scrapeFail", a.scrapeFailure.Load()).
			Int64("queueRejected", a.queueRejected.Load()).
			Int("uniqueAPIKeys", uniqueKeys).
			Msg("analytics")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
