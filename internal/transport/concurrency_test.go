package transport_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransport_WithConcurrency_limitsInFlight(t *testing.T) {
	const limit = 2
	const total = 5

	var inFlight atomic.Int32
	var maxSeen atomic.Int32

	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		cur := inFlight.Add(1)
		defer inFlight.Add(-1)

		// track peak concurrency
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond) // hold slot long enough to overlap
		w.Write([]byte("{}"))
	})
	defer srv.Close()

	tr = tr.WithConcurrency(limit)

	var wg sync.WaitGroup
	errs := make([]error, total)
	for i := range total {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = tr.Do(context.Background(), "GET", "/", nil, nil)
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if got := maxSeen.Load(); got > limit {
		t.Errorf("concurrency exceeded limit %d: peak was %d", limit, got)
	}
}

func TestTransport_WithConcurrency_contextCancelledWhileWaiting(t *testing.T) {
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("{}"))
	})
	defer srv.Close()

	tr = tr.WithConcurrency(1)

	// Fill the single slot.
	go tr.Do(context.Background(), "GET", "/", nil, nil) //nolint:errcheck
	time.Sleep(10 * time.Millisecond)                    // let goroutine acquire slot

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := tr.Do(ctx, "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected error when context cancelled while waiting for semaphore")
	}
}
