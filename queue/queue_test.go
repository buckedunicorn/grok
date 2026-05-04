package queue_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/queue"
)

func TestQueue_rateLimits(t *testing.T) {
	q := queue.New(10) // 10 rps = 100ms between calls

	start := time.Now()
	for range 3 {
		if err := q.Submit(context.Background(), func() error { return nil }); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	elapsed := time.Since(start)

	// 3 calls at 10 rps: first is immediate, then 2 × 100ms minimum wait
	if elapsed < 180*time.Millisecond {
		t.Errorf("rate limit too fast: %v (expected >= 180ms)", elapsed)
	}
}

func TestQueue_contextCancelled(t *testing.T) {
	q := queue.New(0.5) // very slow, 2 seconds between calls

	// First call consumes the slot immediately.
	q.Submit(context.Background(), func() error { return nil }) //nolint:errcheck

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := q.Submit(ctx, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestQueue_submitAll_stopsOnError(t *testing.T) {
	q := queue.New(100) // fast

	var called atomic.Int32
	err := q.SubmitAll(context.Background(),
		func() error { called.Add(1); return nil },
		func() error { called.Add(1); return errors.New("boom") },
		func() error { called.Add(1); return nil }, // should not run
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if called.Load() != 2 {
		t.Errorf("expected 2 calls before error, got %d", called.Load())
	}
}

func TestQueue_zero_unlimited(t *testing.T) {
	q := queue.New(0) // unlimited

	start := time.Now()
	for range 5 {
		q.Submit(context.Background(), func() error { return nil }) //nolint:errcheck
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("unlimited queue should be near-instant, took %v", elapsed)
	}
}
