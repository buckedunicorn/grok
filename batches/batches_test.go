package batches_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/batches"
	"github.com/buckedunicorn/grok/internal/transport"
)

func newBatchesClient(handler http.HandlerFunc) (*httptest.Server, *batches.Client) {
	srv := httptest.NewServer(handler)
	t := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, batches.NewClient(t)
}

// --- All ---

func TestBatches_All_singlePage(t *testing.T) {
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(batches.BatchList{
			Batches: []batches.Batch{
				{BatchID: "b1"},
				{BatchID: "b2"},
			},
		})
	})
	defer srv.Close()

	var got []string
	for b, err := range c.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, b.BatchID)
	}
	if len(got) != 2 || got[0] != "b1" || got[1] != "b2" {
		t.Errorf("unexpected batches: %v", got)
	}
}

func TestBatches_All_multiPage(t *testing.T) {
	pages := 0
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		pages++
		tok := r.URL.Query().Get("pagination_token")
		switch tok {
		case "":
			next := "pg2"
			json.NewEncoder(w).Encode(batches.BatchList{
				Batches:         []batches.Batch{{BatchID: "b1"}},
				PaginationToken: &next,
			})
		case "pg2":
			json.NewEncoder(w).Encode(batches.BatchList{
				Batches: []batches.Batch{{BatchID: "b2"}},
			})
		default:
			t.Errorf("unexpected pagination_token: %s", tok)
			w.WriteHeader(500)
		}
	})
	defer srv.Close()

	var got []string
	for b, err := range c.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, b.BatchID)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 batches, got %d: %v", len(got), got)
	}
	if pages != 2 {
		t.Errorf("expected 2 page fetches, got %d", pages)
	}
}

func TestBatches_All_earlyBreak(t *testing.T) {
	calls := 0
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		next := "pg2"
		json.NewEncoder(w).Encode(batches.BatchList{
			Batches:         []batches.Batch{{BatchID: "b1"}, {BatchID: "b2"}},
			PaginationToken: &next,
		})
	})
	defer srv.Close()

	count := 0
	for _, err := range c.All(context.Background(), nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count++
		break
	}
	if count != 1 {
		t.Errorf("expected 1 item before break, got %d", count)
	}
	if calls != 1 {
		t.Errorf("expected 1 HTTP call, got %d", calls)
	}
}

// --- AllRequests ---

func TestBatches_AllRequests_multiPage(t *testing.T) {
	pages := 0
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		pages++
		tok := r.URL.Query().Get("pagination_token")
		switch tok {
		case "":
			next := "pg2"
			json.NewEncoder(w).Encode(batches.RequestList{
				BatchRequestMetadata: []batches.RequestMetadata{{BatchRequestID: "r1"}},
				PaginationToken:      &next,
			})
		case "pg2":
			json.NewEncoder(w).Encode(batches.RequestList{
				BatchRequestMetadata: []batches.RequestMetadata{{BatchRequestID: "r2"}},
			})
		default:
			t.Errorf("unexpected pagination_token: %s", tok)
			w.WriteHeader(500)
		}
	})
	defer srv.Close()

	var got []string
	for rm, err := range c.AllRequests(context.Background(), "batch-1", nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, rm.BatchRequestID)
	}
	if len(got) != 2 || got[0] != "r1" || got[1] != "r2" {
		t.Errorf("unexpected requests: %v", got)
	}
	if pages != 2 {
		t.Errorf("expected 2 page fetches, got %d", pages)
	}
}

// --- AllResults ---

func TestBatches_AllResults_multiPage(t *testing.T) {
	pages := 0
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		pages++
		tok := r.URL.Query().Get("pagination_token")
		switch tok {
		case "":
			next := "pg2"
			json.NewEncoder(w).Encode(batches.ResultList{
				Results:         []batches.BatchResult{{BatchRequestID: "res1"}},
				PaginationToken: &next,
			})
		case "pg2":
			json.NewEncoder(w).Encode(batches.ResultList{
				Results: []batches.BatchResult{{BatchRequestID: "res2"}},
			})
		default:
			t.Errorf("unexpected pagination_token: %s", tok)
			w.WriteHeader(500)
		}
	})
	defer srv.Close()

	var got []string
	for br, err := range c.AllResults(context.Background(), "batch-1", nil) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got = append(got, br.BatchRequestID)
	}
	if len(got) != 2 || got[0] != "res1" || got[1] != "res2" {
		t.Errorf("unexpected results: %v", got)
	}
	if pages != 2 {
		t.Errorf("expected 2 page fetches, got %d", pages)
	}
}

// --- Wait ---

func TestBatches_Wait_alreadyDone(t *testing.T) {
	calls := 0
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(batches.Batch{
			BatchID: "b1",
			State:   batches.BatchState{NumPending: 0, NumSuccess: 2},
		})
	})
	defer srv.Close()

	b, err := c.Wait(context.Background(), "b1", time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.BatchID != "b1" {
		t.Errorf("unexpected batch ID: %s", b.BatchID)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestBatches_Wait_pollsUntilDone(t *testing.T) {
	var calls atomic.Int32
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		pending := 1
		if n >= 3 {
			pending = 0
		}
		json.NewEncoder(w).Encode(batches.Batch{
			BatchID: "b1",
			State:   batches.BatchState{NumPending: pending},
		})
	})
	defer srv.Close()

	b, err := c.Wait(context.Background(), "b1", time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.State.NumPending != 0 {
		t.Errorf("expected no pending, got %d", b.State.NumPending)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

func TestBatches_Wait_contextCancelled(t *testing.T) {
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(batches.Batch{
			BatchID: "b1",
			State:   batches.BatchState{NumPending: 1},
		})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := c.Wait(ctx, "b1", 5*time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
}

func TestBatches_Wait_apiError(t *testing.T) {
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":"internal","error":"server error"}`))
	})
	defer srv.Close()

	_, err := c.Wait(context.Background(), "b1", time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBatches_Wait_defaultInterval(t *testing.T) {
	// Passing 0 should not panic and should use the default interval.
	// We cancel immediately after the first successful poll to avoid a 10s wait.
	calls := 0
	srv, c := newBatchesClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		json.NewEncoder(w).Encode(batches.Batch{
			BatchID: "b1",
			State:   batches.BatchState{NumPending: 0},
		})
	})
	defer srv.Close()

	b, err := c.Wait(context.Background(), "b1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil batch")
	}
}
