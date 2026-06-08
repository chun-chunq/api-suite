// Package scrapequeue wraps the browser semaphore with a bounded request queue.
//
// When all Chrome slots are busy, incoming requests wait in the queue.
// When the queue depth exceeds maxDepth, requests get an immediate 429 with
// position info and a Retry-After estimate instead of hanging forever.
package scrapequeue

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// ErrQueueFull is returned when the pending queue is at capacity.
var ErrQueueFull = errors.New("scrape queue full")

// Queue serialises Chrome access and tracks depth/timing metrics.
type Queue struct {
	slots    chan struct{} // pre-filled semaphore; receive = acquire a browser
	maxDepth int64

	// metrics — all atomic, zero-cost on hot path
	pendingWaiters atomic.Int64
	maxDepthSeen   atomic.Int64
	totalQueued    atomic.Int64
	totalFulfilled atomic.Int64
	totalTimedOut  atomic.Int64
	totalRejected  atomic.Int64
	totalWaitMs    atomic.Int64 // sum of queue wait durations
}

// New creates a Queue with the given number of browser slots and queue depth limit.
// maxBrowsers = max concurrent Chrome instances
// maxDepth    = max requests allowed to wait before returning 429
func New(maxBrowsers, maxDepth int) *Queue {
	if maxBrowsers < 1 {
		maxBrowsers = 1
	}
	if maxDepth < 1 {
		maxDepth = 20
	}
	slots := make(chan struct{}, maxBrowsers)
	for i := 0; i < maxBrowsers; i++ {
		slots <- struct{}{}
	}
	return &Queue{slots: slots, maxDepth: int64(maxDepth)}
}

// Acquire blocks until a Chrome slot is available or the context is cancelled.
// Returns a release function that must be called when the scrape is done,
// the current queue position, and the time spent waiting.
//
// Returns ErrQueueFull when the pending count exceeds maxDepth.
func (q *Queue) Acquire(ctx context.Context) (release func(), waitDur time.Duration, err error) {
	depth := q.pendingWaiters.Add(1)

	if depth > q.maxDepth {
		q.pendingWaiters.Add(-1)
		q.totalRejected.Add(1)
		return nil, 0, ErrQueueFull
	}

	// Update peak depth seen (lock-free CAS loop)
	for {
		old := q.maxDepthSeen.Load()
		if depth <= old {
			break
		}
		if q.maxDepthSeen.CompareAndSwap(old, depth) {
			break
		}
	}

	q.totalQueued.Add(1)
	queued := time.Now()

	select {
	case <-q.slots: // got a browser slot
		wait := time.Since(queued)
		q.pendingWaiters.Add(-1)
		q.totalWaitMs.Add(wait.Milliseconds())
		return func() { q.slots <- struct{}{} }, wait, nil

	case <-ctx.Done():
		q.pendingWaiters.Add(-1)
		q.totalTimedOut.Add(1)
		return nil, time.Since(queued), ctx.Err()
	}
}

// Complete records a successfully fulfilled scrape.
func (q *Queue) Complete() { q.totalFulfilled.Add(1) }

// Depth returns the current number of requests waiting for a Chrome slot.
func (q *Queue) Depth() int { return int(q.pendingWaiters.Load()) }

// Capacity returns the number of Chrome slots.
func (q *Queue) Capacity() int { return cap(q.slots) }

// AvailableSlots returns how many Chrome instances are free right now.
func (q *Queue) AvailableSlots() int { return len(q.slots) }

// Stats returns a snapshot of queue metrics. Safe to call from any goroutine.
func (q *Queue) Stats() map[string]any {
	queued := q.totalQueued.Load()
	fulfilled := q.totalFulfilled.Load()
	var avgWaitMs float64
	if fulfilled > 0 {
		avgWaitMs = float64(q.totalWaitMs.Load()) / float64(fulfilled)
	}
	return map[string]any{
		"capacity":       q.Capacity(),
		"availableSlots": q.AvailableSlots(),
		"currentDepth":   q.Depth(),
		"maxDepthSeen":   q.maxDepthSeen.Load(),
		"maxDepth":       q.maxDepth,
		"totalQueued":    queued,
		"totalFulfilled": fulfilled,
		"totalTimedOut":  q.totalTimedOut.Load(),
		"totalRejected":  q.totalRejected.Load(),
		"avgWaitMs":      avgWaitMs,
	}
}

// RetryAfterSeconds estimates how long a caller should wait before retrying.
// Based on current depth and assumed average scrape duration.
func (q *Queue) RetryAfterSeconds(assumedScrapeSec int) int {
	depth := q.Depth()
	slots := q.Capacity()
	if slots < 1 {
		slots = 1
	}
	slotsNeeded := (depth + slots - 1) / slots
	return slotsNeeded * assumedScrapeSec
}
