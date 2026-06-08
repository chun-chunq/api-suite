// Package jobqueue implements a simple in-memory job queue for the PC-Worker bridge.
// The home-PC worker long-polls /internal/worker/poll, executes scrapes, and
// POSTs results back to /internal/worker/result/{id}.
package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrTimeout is returned by Dispatch when no worker picks up the job in time.
var ErrTimeout = errors.New("no PC-worker responded in time")

// ErrNoWorker is returned when there are no workers currently polling.
var ErrNoWorker = errors.New("no PC-worker connected")

const jobTTL = 90 * time.Second

// Job is a pending scrape request placed by an API handler.
type Job struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`    // "insolvency" | "zvg"
	Payload   json.RawMessage `json:"payload"` // search query JSON
	resultCh  chan result
	createdAt time.Time
}

type result struct {
	data json.RawMessage
	err  error
}

// Queue holds pending jobs and worker subscribers.
type Queue struct {
	mu      sync.Mutex
	pending []*Job          // jobs waiting to be claimed
	notify  []chan struct{}  // one channel per polling worker session
}

// New creates an empty Queue.
func New() *Queue {
	return &Queue{}
}

// Dispatch creates a job and blocks until a PC-worker returns a result or the
// context deadline is reached.
func (q *Queue) Dispatch(ctx context.Context, jobType string, payload any) (json.RawMessage, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	job := &Job{
		ID:        uuid.NewString(),
		Type:      jobType,
		Payload:   payloadJSON,
		resultCh:  make(chan result, 1),
		createdAt: time.Now(),
	}

	q.mu.Lock()
	if len(q.notify) == 0 {
		q.mu.Unlock()
		return nil, ErrNoWorker
	}
	q.pending = append(q.pending, job)
	// wake up all waiting workers
	for _, ch := range q.notify {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	q.mu.Unlock()

	// Wait for result
	waitCtx, cancel := context.WithTimeout(ctx, jobTTL)
	defer cancel()
	select {
	case res := <-job.resultCh:
		return res.data, res.err
	case <-waitCtx.Done():
		q.removeJob(job.ID)
		return nil, ErrTimeout
	}
}

// Poll blocks until a job is available for the given types or the context expires.
// Returns nil if context expired (long-poll timeout → 204).
func (q *Queue) Poll(ctx context.Context, types map[string]bool) *Job {
	notify := make(chan struct{}, 1)

	q.mu.Lock()
	// Check for already-pending jobs before blocking
	if job := q.claimJob(types); job != nil {
		q.mu.Unlock()
		return job
	}
	q.notify = append(q.notify, notify)
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		for i, ch := range q.notify {
			if ch == notify {
				q.notify = append(q.notify[:i], q.notify[i+1:]...)
				break
			}
		}
		q.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-notify:
			q.mu.Lock()
			job := q.claimJob(types)
			q.mu.Unlock()
			if job != nil {
				return job
			}
		}
	}
}

// Complete delivers a result for the job with the given ID.
func (q *Queue) Complete(id string, data json.RawMessage, jobErr string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, job := range q.pending {
		if job.ID == id {
			var err error
			if jobErr != "" {
				err = errors.New(jobErr)
			}
			select {
			case job.resultCh <- result{data: data, err: err}:
			default:
			}
			return true
		}
	}
	return false
}

// HasWorkers returns true when at least one PC-worker is currently polling.
func (q *Queue) HasWorkers() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.notify) > 0
}

// claimJob removes and returns the first pending job matching the given types.
// Must be called with q.mu held.
func (q *Queue) claimJob(types map[string]bool) *Job {
	for i, job := range q.pending {
		if types[job.Type] {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return job
		}
	}
	return nil
}

func (q *Queue) removeJob(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, job := range q.pending {
		if job.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return
		}
	}
}
