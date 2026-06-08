package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

var errFail = errors.New("upstream failed")

func TestBreaker_ClosedToOpen(t *testing.T) {
	b := New("test", Config{ConsecutiveFailures: 3, Timeout: time.Second})

	// 3 failures should open the breaker
	for i := 0; i < 3; i++ {
		ok, _ := b.Allow()
		if !ok {
			t.Fatalf("should allow request %d", i)
		}
		b.RecordFailure()
	}

	if b.GetState() != StateOpen {
		t.Errorf("expected Open, got %s", b.GetState())
	}

	// Now requests should be rejected
	_, err := b.Allow()
	if err == nil {
		t.Error("expected error when circuit is open")
	}
	if !errors.Is(err, ErrOpen) {
		t.Errorf("expected ErrOpen, got: %v", err)
	}
}

func TestBreaker_OpenToHalfOpen(t *testing.T) {
	b := New("test", Config{ConsecutiveFailures: 1, Timeout: 50 * time.Millisecond})

	ok, _ := b.Allow()
	if !ok {
		t.Fatal("should allow first request")
	}
	b.RecordFailure()

	if b.GetState() != StateOpen {
		t.Error("expected Open")
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open
	ok, err := b.Allow()
	if !ok || err != nil {
		t.Errorf("expected probe allowed, got ok=%v err=%v", ok, err)
	}
	if b.GetState() != StateHalfOpen {
		t.Errorf("expected HalfOpen, got %s", b.GetState())
	}
}

func TestBreaker_HalfOpenSuccessCloses(t *testing.T) {
	b := New("test", Config{ConsecutiveFailures: 1, Timeout: 50 * time.Millisecond})
	b.Allow()
	b.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	b.Allow() // transitions to half-open
	b.RecordSuccess()

	if b.GetState() != StateClosed {
		t.Errorf("expected Closed after successful probe, got %s", b.GetState())
	}
}

func TestBreaker_HalfOpenFailureReopens(t *testing.T) {
	b := New("test", Config{ConsecutiveFailures: 1, Timeout: 50 * time.Millisecond})
	b.Allow()
	b.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	b.Allow() // transitions to half-open
	b.RecordFailure()

	if b.GetState() != StateOpen {
		t.Errorf("expected Open after failed probe, got %s", b.GetState())
	}
}

func TestBreaker_SuccessResetsFailCount(t *testing.T) {
	b := New("test", Config{ConsecutiveFailures: 5, Timeout: time.Second})

	for i := 0; i < 4; i++ {
		b.Allow()
		b.RecordFailure()
	}

	// One success resets the counter
	b.Allow()
	b.RecordSuccess()

	if b.GetState() != StateClosed {
		t.Error("expected Closed")
	}
	if b.fails != 0 {
		t.Errorf("expected fails=0 after success, got %d", b.fails)
	}
}

func TestRegistry_Do(t *testing.T) {
	r := NewRegistry(Config{ConsecutiveFailures: 3, Timeout: time.Second})

	// Two successful calls
	for i := 0; i < 2; i++ {
		err := r.Do("myservice", func() error { return nil })
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Three failures open the breaker
	for i := 0; i < 3; i++ {
		r.Do("myservice", func() error { return errFail })
	}

	// Next call should fail fast
	err := r.Do("myservice", func() error { return nil })
	if !errors.Is(err, ErrOpen) {
		t.Errorf("expected ErrOpen, got %v", err)
	}
}

func TestRegistry_AllStats(t *testing.T) {
	r := NewRegistry(DefaultConfig())
	r.Get("api-a")
	r.Get("api-b")
	r.Get("api-c")

	stats := r.AllStats()
	if len(stats) != 3 {
		t.Errorf("expected 3 stats, got %d", len(stats))
	}
}

func TestBreaker_Stats_OpenShowsRetryIn(t *testing.T) {
	b := New("test", Config{ConsecutiveFailures: 1, Timeout: 30 * time.Second})
	b.Allow()
	b.RecordFailure()

	stats := b.Stats()
	if stats["state"] != "open" {
		t.Errorf("expected state=open, got %v", stats["state"])
	}
	if _, ok := stats["retry_in_seconds"]; !ok {
		t.Error("expected retry_in_seconds in open state stats")
	}
}
