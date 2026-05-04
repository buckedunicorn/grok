// Package queue provides a token-bucket rate limiter for API calls.
//
// Use New(rps) to create a Queue that limits outgoing requests to at most
// rps requests per second. Submit and SubmitAll honour context cancellation.
//
//	q := queue.New(2.0) // 2 requests/second
//	for _, prompt := range prompts {
//	 err := q.Submit(ctx, func() error { return sendRequest(prompt) })
//	}
package queue

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Queue serialises API calls to stay within a requests-per-second budget.
// It uses a leaky-bucket approach: each call waits until its scheduled slot.
type Queue struct {
	interval time.Duration
	mu       sync.Mutex
	next     time.Time // earliest time the next request may fire
	maxAhead time.Duration
}

// Option configures a Queue.
type Option func(*Queue)

// WithMaxQueueAhead caps how far in the future the next slot may
// schedule. When a Submit call would push the next slot past this
// horizon (relative to now), the call returns ErrQueueFull immediately
// instead of waiting. Pass <= 0 to disable (default).
func WithMaxQueueAhead(d time.Duration) Option {
	return func(q *Queue) { q.maxAhead = d }
}

// ErrQueueFull is returned by Submit when the queue's WithMaxQueueAhead
// horizon would be exceeded.
var ErrQueueFull = errors.New("queue: ahead horizon exceeded")

// New creates a Queue limited to rps requests per second.
// rps must be positive; values <= 0 are treated as unlimited (no delay).
func New(rps float64, opts ...Option) *Queue {
	q := &Queue{}
	if rps > 0 {
		q.interval = time.Duration(float64(time.Second) / rps)
	}
	for _, o := range opts {
		o(q)
	}
	return q
}

// Submit waits for a rate-limit slot and then calls fn.
// Returns fn's error, or ctx.Err() if the context is cancelled while waiting.
func (q *Queue) Submit(ctx context.Context, fn func() error) error {
	if err := q.wait(ctx); err != nil {
		return err
	}
	return fn()
}

// SubmitAll calls each fn in order, rate-limited. Stops at the first error.
func (q *Queue) SubmitAll(ctx context.Context, fns ...func() error) error {
	for _, fn := range fns {
		if err := q.Submit(ctx, fn); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) wait(ctx context.Context) error {
	if q.interval == 0 {
		return ctx.Err()
	}
	q.mu.Lock()
	now := time.Now()
	if q.next.IsZero() || now.After(q.next) {
		q.next = now.Add(q.interval)
		q.mu.Unlock()
		return nil
	}
	wait := q.next.Sub(now)
	if q.maxAhead > 0 && wait > q.maxAhead {
		q.mu.Unlock()
		return ErrQueueFull
	}
	prevNext := q.next
	q.next = q.next.Add(q.interval)
	q.mu.Unlock()

	t := time.NewTimer(wait)
	select {
	case <-ctx.Done():
		t.Stop()
		// Roll back the reserved slot if no other caller has scheduled
		// past us in the meantime. Without this, every
		// cancelled wait permanently shifts q.next forward by interval
		// and degrades the queue's effective rate.
		q.mu.Lock()
		if q.next.Equal(prevNext.Add(q.interval)) {
			q.next = prevNext
		}
		q.mu.Unlock()
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
