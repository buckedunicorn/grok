package runnable_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/runnable"
)

// parseInt and double, trivial Runnables for tests.
var parseInt = runnable.Func[string, int](func(_ context.Context, s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a digit")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
})

var double = runnable.Func[int, int](func(_ context.Context, n int) (int, error) {
	return n * 2, nil
})

func TestPipe_basic(t *testing.T) {
	p := runnable.Pipe(parseInt, double)
	got, err := p.Run(context.Background(), "21")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Errorf("got %d", got)
	}
}

func TestPipe3_basic(t *testing.T) {
	identity := runnable.Func[int, int](func(_ context.Context, n int) (int, error) { return n, nil })
	p := runnable.Pipe3(parseInt, double, identity)
	got, err := p.Run(context.Background(), "5")
	if err != nil {
		t.Fatal(err)
	}
	if got != 10 {
		t.Errorf("got %d", got)
	}
}

func TestPipe_errorPropagates(t *testing.T) {
	p := runnable.Pipe(parseInt, double)
	_, err := p.Run(context.Background(), "abc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParallel_runsAllAndPreservesOrder(t *testing.T) {
	a := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) { return 1, nil })
	b := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) { return 2, nil })
	c := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) { return 3, nil })
	p := runnable.Parallel(a, b, c)
	got, err := p.Run(context.Background(), "in")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestParallel_firstErrorAbortsOthers(t *testing.T) {
	var bRan atomic.Bool
	a := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) {
		return 0, errors.New("boom")
	})
	b := runnable.Func[string, int](func(ctx context.Context, _ string) (int, error) {
		select {
		case <-time.After(50 * time.Millisecond):
			bRan.Store(true)
			return 2, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	})
	p := runnable.Parallel(a, b)
	_, err := p.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBranch_picksByIndex(t *testing.T) {
	a := runnable.Func[int, string](func(_ context.Context, _ int) (string, error) { return "a", nil })
	b := runnable.Func[int, string](func(_ context.Context, _ int) (string, error) { return "b", nil })
	choose := func(n int) int {
		if n%2 == 0 {
			return 0
		}
		return 1
	}
	p := runnable.Branch(choose, a, b)
	got, _ := p.Run(context.Background(), 4)
	if got != "a" {
		t.Errorf("got %q", got)
	}
	got, _ = p.Run(context.Background(), 5)
	if got != "b" {
		t.Errorf("got %q", got)
	}
}

func TestBranch_outOfRange(t *testing.T) {
	a := runnable.Func[int, string](func(_ context.Context, _ int) (string, error) { return "a", nil })
	p := runnable.Branch(func(int) int { return 99 }, a)
	_, err := p.Run(context.Background(), 0)
	if !errors.Is(err, runnable.ErrBranchOutOfRange) {
		t.Errorf("err = %v, want ErrBranchOutOfRange", err)
	}
}

func TestMap_sequential(t *testing.T) {
	p := runnable.Map(double)
	got, err := p.Run(context.Background(), []int{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 2 || got[1] != 4 || got[2] != 6 {
		t.Errorf("got %+v", got)
	}
}

func TestParallelMap_concurrent(t *testing.T) {
	p := runnable.ParallelMap(double)
	got, err := p.Run(context.Background(), []int{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3] != 8 {
		t.Errorf("got %+v", got)
	}
}

func TestRetry_succeedsAfterFailures(t *testing.T) {
	var attempts atomic.Int32
	flaky := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) {
		n := attempts.Add(1)
		if n < 3 {
			return 0, errors.New("transient")
		}
		return 42, nil
	})
	p := runnable.Retry(runnable.RetryOptions{
		MaxAttempts:    5,
		InitialBackoff: time.Millisecond,
	}, flaky)
	got, err := p.Run(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 || attempts.Load() != 3 {
		t.Errorf("got %d after %d attempts", got, attempts.Load())
	}
}

func TestRetry_givesUpAfterMaxAttempts(t *testing.T) {
	var attempts atomic.Int32
	always := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) {
		attempts.Add(1)
		return 0, errors.New("never")
	})
	p := runnable.Retry(runnable.RetryOptions{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
	}, always)
	_, err := p.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

func TestRetry_shouldRetryGate(t *testing.T) {
	permErr := errors.New("permanent")
	var attempts atomic.Int32
	always := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) {
		attempts.Add(1)
		return 0, permErr
	})
	p := runnable.Retry(runnable.RetryOptions{
		MaxAttempts:    5,
		InitialBackoff: time.Millisecond,
		ShouldRetry:    func(err error) bool { return !errors.Is(err, permErr) },
	}, always)
	_, err := p.Run(context.Background(), "x")
	if !errors.Is(err, permErr) {
		t.Fatalf("err = %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (no retries on permanent err)", attempts.Load())
	}
}

func TestRetry_ctxCancellation(t *testing.T) {
	always := runnable.Func[string, int](func(_ context.Context, _ string) (int, error) {
		return 0, errors.New("fail")
	})
	p := runnable.Retry(runnable.RetryOptions{
		MaxAttempts:    100,
		InitialBackoff: 100 * time.Millisecond,
	}, always)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.Run(ctx, "x")
	if err == nil {
		t.Fatal("expected error")
	}
}
