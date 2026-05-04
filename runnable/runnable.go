// Package runnable provides a generic Runnable[In, Out] interface and small
// combinators (Pipe, Parallel, Branch, Map, Retry) for composing typed
// pipelines.
//
// Runnable is the SDK's universal connector: prompts, chat calls, parsers,
// and tools can all satisfy the interface, so they compose without glue
// code. The package is opt-in, using *chat.Client directly remains the
// simplest path. Reach for runnable when you have a multi-step pipeline
// you want typed end-to-end:
//
//	type Summary struct{ Text string }
//
//	render := runnable.Func[map[string]any, []chat.Message](func(_ context.Context, vars map[string]any) ([]chat.Message, error) {
//	 return tmpl.Render(vars)
//	})
//	call := runnable.Func[[]chat.Message, *chat.Completion](func(ctx context.Context, msgs []chat.Message) (*chat.Completion, error) {
//	 return client.Chat.Create(ctx, &chat.CreateRequest{Model: "...", Messages: msgs})
//	})
//	parse := runnable.Func[*chat.Completion, Summary](func(_ context.Context, c *chat.Completion) (Summary, error) {
//	 return chat.Decode[Summary](c)
//	})
//
//	pipeline := runnable.Pipe3(render, call, parse)
//	result, err := pipeline.Run(ctx, map[string]any{"text": "..."})
package runnable

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"
)

// Runnable is the universal connector: anything that takes In and produces
// Out via context-aware execution.
type Runnable[In, Out any] interface {
	Run(ctx context.Context, in In) (Out, error)
}

// Func adapts a plain function into a Runnable.
type Func[In, Out any] func(ctx context.Context, in In) (Out, error)

// Run satisfies Runnable for Func.
func (f Func[In, Out]) Run(ctx context.Context, in In) (Out, error) {
	return f(ctx, in)
}

// --- Pipe (sequential composition) ---

// Pipe composes A -> B sequentially: out := b(a(in)).
func Pipe[A, B, C any](a Runnable[A, B], b Runnable[B, C]) Runnable[A, C] {
	return Func[A, C](func(ctx context.Context, in A) (C, error) {
		var zero C
		mid, err := a.Run(ctx, in)
		if err != nil {
			return zero, err
		}
		return b.Run(ctx, mid)
	})
}

// Pipe3 composes three Runnables A -> B -> C -> D. Sugar for nested Pipe.
func Pipe3[A, B, C, D any](a Runnable[A, B], b Runnable[B, C], c Runnable[C, D]) Runnable[A, D] {
	return Pipe(Pipe(a, b), c)
}

// Pipe4 composes four Runnables.
func Pipe4[A, B, C, D, E any](a Runnable[A, B], b Runnable[B, C], c Runnable[C, D], d Runnable[D, E]) Runnable[A, E] {
	return Pipe(Pipe3(a, b, c), d)
}

// --- Parallel ---

// Parallel runs every Runnable in rs concurrently with the same input and
// returns their outputs in input order. The first error aborts pending work
// (via context cancellation) and is returned.
func Parallel[In, Out any](rs ...Runnable[In, Out]) Runnable[In, []Out] {
	return Func[In, []Out](func(ctx context.Context, in In) ([]Out, error) {
		results := make([]Out, len(rs))
		errs := make([]error, len(rs))
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var wg sync.WaitGroup
		for i, r := range rs {
			wg.Add(1)
			go func(i int, r Runnable[In, Out]) {
				defer wg.Done()
				v, err := r.Run(ctx, in)
				if err != nil {
					errs[i] = err
					cancel()
					return
				}
				results[i] = v
			}(i, r)
		}
		wg.Wait()

		// Return the first non-context error so the caller sees the
		// underlying cause rather than a context.Canceled from a
		// faster-cancelling goroutine.
		if err := firstNonContextErr(errs); err != nil {
			return nil, err
		}
		return results, nil
	})
}

// firstNonContextErr returns the first error in errs that is not a
// context cancellation. If every error is a context cancellation,
// returns the first one. Returns nil if errs is all-nil.
func firstNonContextErr(errs []error) error {
	var ctxErr error
	for _, e := range errs {
		if e == nil {
			continue
		}
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
			if ctxErr == nil {
				ctxErr = e
			}
			continue
		}
		return e
	}
	return ctxErr
}

// --- Branch ---

// Branch picks one of rs based on the index returned by choose. If choose
// returns an out-of-range index, Branch returns ErrBranchOutOfRange.
var ErrBranchOutOfRange = errors.New("runnable: Branch index out of range")

// Branch runs the Runnable at rs[choose(in)] and returns its result.
func Branch[In, Out any](choose func(In) int, rs ...Runnable[In, Out]) Runnable[In, Out] {
	return Func[In, Out](func(ctx context.Context, in In) (Out, error) {
		var zero Out
		idx := choose(in)
		if idx < 0 || idx >= len(rs) {
			return zero, fmt.Errorf("%w: index %d, len %d", ErrBranchOutOfRange, idx, len(rs))
		}
		return rs[idx].Run(ctx, in)
	})
}

// --- Map ---

// Map applies r to each element of the input slice and returns the results
// in order. Elements are processed sequentially. Use ParallelMap for
// concurrent execution.
func Map[In, Out any](r Runnable[In, Out]) Runnable[[]In, []Out] {
	return Func[[]In, []Out](func(ctx context.Context, ins []In) ([]Out, error) {
		out := make([]Out, len(ins))
		for i, in := range ins {
			v, err := r.Run(ctx, in)
			if err != nil {
				return nil, fmt.Errorf("runnable.Map[%d]: %w", i, err)
			}
			out[i] = v
		}
		return out, nil
	})
}

// MapOption configures ParallelMap.
type MapOption func(*mapConfig)

type mapConfig struct {
	maxConcurrency int
}

// WithMapConcurrency caps the number of goroutines ParallelMap may
// have running at once. <= 0 means "one goroutine per input"
// (historical behaviour). For very large inputs (10k+) set a finite
// concurrency to avoid spawning that many goroutines at once
// .
func WithMapConcurrency(n int) MapOption {
	return func(c *mapConfig) { c.maxConcurrency = n }
}

// ParallelMap is the concurrent variant of Map. The first element error
// cancels remaining work via the shared context.
func ParallelMap[In, Out any](r Runnable[In, Out], opts ...MapOption) Runnable[[]In, []Out] {
	cfg := &mapConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return Func[[]In, []Out](func(ctx context.Context, ins []In) ([]Out, error) {
		out := make([]Out, len(ins))
		errs := make([]error, len(ins))
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var sem chan struct{}
		if cfg.maxConcurrency > 0 {
			sem = make(chan struct{}, cfg.maxConcurrency)
		}

		var wg sync.WaitGroup
		for i, in := range ins {
			if sem != nil {
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					// Stop scheduling new work once ctx fires.
					goto wait
				}
			}
			wg.Add(1)
			go func(i int, in In) {
				defer wg.Done()
				if sem != nil {
					defer func() { <-sem }()
				}
				v, err := r.Run(ctx, in)
				if err != nil {
					errs[i] = fmt.Errorf("runnable.ParallelMap[%d]: %w", i, err)
					cancel()
					return
				}
				out[i] = v
			}(i, in)
		}
	wait:
		wg.Wait()

		// Return the first non-context error.
		if err := firstNonContextErr(errs); err != nil {
			return nil, err
		}
		return out, nil
	})
}

// --- Retry ---

// RetryOptions configures Retry. MaxAttempts is the total number of tries
// (including the initial one); 0 or negative means a single attempt.
// InitialBackoff is the delay before the second attempt; subsequent delays
// double. MaxBackoff caps the per-attempt delay (default 30s); 0 or negative
// disables the cap (not recommended; see ). Jitter (0..1)
// randomizes each delay by up to ±jitter*delay. ShouldRetry returns true to
// retry on a given error; nil retries on every non-nil error.
type RetryOptions struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Jitter         float64
	ShouldRetry    func(error) bool
}

// Retry wraps r with bounded exponential backoff retries per opts. The
// underlying context applies to every attempt; cancellation aborts pending
// retries immediately. The per-attempt delay is capped at MaxBackoff so a
// large MaxAttempts cannot wedge the goroutine for hours.
func Retry[In, Out any](opts RetryOptions, r Runnable[In, Out]) Runnable[In, Out] {
	if opts.MaxAttempts < 1 {
		opts.MaxAttempts = 1
	}
	if opts.InitialBackoff <= 0 {
		opts.InitialBackoff = 100 * time.Millisecond
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 30 * time.Second
	}
	if opts.Jitter < 0 {
		opts.Jitter = 0
	} else if opts.Jitter > 1 {
		opts.Jitter = 1
	}
	return Func[In, Out](func(ctx context.Context, in In) (Out, error) {
		var zero Out
		var lastErr error
		delay := opts.InitialBackoff

		for attempt := 0; attempt < opts.MaxAttempts; attempt++ {
			v, err := r.Run(ctx, in)
			if err == nil {
				return v, nil
			}
			lastErr = err
			if opts.ShouldRetry != nil && !opts.ShouldRetry(err) {
				return zero, err
			}
			if attempt == opts.MaxAttempts-1 {
				break
			}

			d := delay
			if opts.Jitter > 0 {
				j := opts.Jitter * (2*rand.Float64() - 1) // [-jitter, +jitter]
				d = max(time.Duration(float64(d)*(1+j)), 0)
			}
			t := time.NewTimer(d)
			select {
			case <-ctx.Done():
				t.Stop()
				return zero, ctx.Err()
			case <-t.C:
			}
			delay = min(delay*2, opts.MaxBackoff)
		}
		return zero, lastErr
	})
}
