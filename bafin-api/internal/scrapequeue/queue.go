package scrapequeue

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

var ErrQueueFull = errors.New("scrape queue full")

type Queue struct {
	slots    chan struct{}
	maxDepth int64
	pending  atomic.Int64
	total    atomic.Int64
	rejected atomic.Int64
}

func New(maxBrowsers, maxDepth int) *Queue {
	q := &Queue{maxDepth: int64(maxDepth), slots: make(chan struct{}, maxBrowsers)}
	for i := 0; i < maxBrowsers; i++ {
		q.slots <- struct{}{}
	}
	return q
}

func (q *Queue) Acquire(ctx context.Context) (release func(), waitDur time.Duration, err error) {
	if q.pending.Load() >= q.maxDepth {
		q.rejected.Add(1)
		return nil, 0, ErrQueueFull
	}
	q.pending.Add(1)
	start := time.Now()
	select {
	case <-q.slots:
		waitDur = time.Since(start)
		q.total.Add(1)
		return func() { q.slots <- struct{}{} }, waitDur, nil
	case <-ctx.Done():
		q.pending.Add(-1)
		return nil, time.Since(start), ctx.Err()
	}
}

func (q *Queue) Complete() { q.pending.Add(-1) }

func (q *Queue) Depth() int { return int(q.pending.Load()) }

func (q *Queue) RetryAfterSeconds(assumedScrapeSec int) int {
	depth := int(q.pending.Load())
	slots := cap(q.slots)
	if slots == 0 {
		return assumedScrapeSec
	}
	return (depth/slots + 1) * assumedScrapeSec
}

func (q *Queue) Stats() map[string]any {
	return map[string]any{
		"pending":  q.pending.Load(),
		"capacity": cap(q.slots),
		"total":    q.total.Load(),
		"rejected": q.rejected.Load(),
	}
}
