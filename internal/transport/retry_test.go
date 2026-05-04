package transport_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/internal/apierr"
)

func TestRetry_succeedsOnFirstAttempt(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte("{}"))
	})
	defer srv.Close()
	tr.MaxRetries = 3

	if err := tr.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", calls.Load())
	}
}

func TestRetry_retriesOn429(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(429)
			w.Write([]byte(`{"code":"rate limit","error":"Too Many Requests"}`))
			return
		}
		w.Write([]byte("{}"))
	})
	defer srv.Close()
	tr.MaxRetries = 3

	if err := tr.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func TestRetry_retriesOn503(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(503)
			w.Write([]byte(`{"code":"unavailable","error":"Service Unavailable"}`))
			return
		}
		w.Write([]byte("{}"))
	})
	defer srv.Close()
	tr.MaxRetries = 2

	if err := tr.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
}

func TestRetry_doesNotRetryOn400(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(400)
		w.Write([]byte(`{"code":"bad request","error":"Bad Request"}`))
	})
	defer srv.Close()
	tr.MaxRetries = 3

	err := tr.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *apierr.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		t.Errorf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("should not retry 400, got %d calls", calls.Load())
	}
}

func TestRetry_doesNotRetryOn401(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(401)
		w.Write([]byte(`{"code":"unauthorized","error":"Unauthorized"}`))
	})
	defer srv.Close()
	tr.MaxRetries = 3

	err := tr.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Errorf("should not retry 401, got %d calls", calls.Load())
	}
}

func TestRetry_exhaustsAndReturnsLastError(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		w.Write([]byte(`{"code":"unavailable","error":"Service Unavailable"}`))
	})
	defer srv.Close()
	tr.MaxRetries = 2

	err := tr.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	var apiErr *apierr.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 503 {
		t.Errorf("unexpected error: %v", err)
	}
	if calls.Load() != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func TestRetry_contextCancelledStopsRetrying(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(429)
		w.Write([]byte(`{"code":"rate limit","error":"Too Many Requests"}`))
	})
	defer srv.Close()
	tr.MaxRetries = 10

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := tr.Do(ctx, "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestRetry_defaultZeroMeansNoRetry(t *testing.T) {
	var calls atomic.Int32
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		w.Write([]byte(`{}`))
	})
	defer srv.Close()
	// MaxRetries defaults to 0, no retry.

	_ = tr.Do(context.Background(), "GET", "/", nil, nil)
	if calls.Load() != 1 {
		t.Errorf("MaxRetries=0 should make exactly 1 call, got %d", calls.Load())
	}
}
