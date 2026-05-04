package videos_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/internal/transport"
	"github.com/buckedunicorn/grok/videos"
)

func newClient(handler http.HandlerFunc) (*httptest.Server, *videos.Client) {
	srv := httptest.NewServer(handler)
	t := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return srv, videos.NewClient(t)
}

func TestWait_returnsDoneImmediately(t *testing.T) {
	srv, c := newClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(videos.VideoResult{
			Status: "done",
			Video:  &videos.VideoData{URL: "https://example.com/video.mp4"},
		})
	})
	defer srv.Close()

	result, err := c.Wait(context.Background(), "req-123", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "done" {
		t.Errorf("Status = %q, want %q", result.Status, "done")
	}
	if result.Video.URL != "https://example.com/video.mp4" {
		t.Errorf("Video.URL = %q", result.Video.URL)
	}
}

func TestWait_pollsUntilDone(t *testing.T) {
	var calls atomic.Int32
	srv, c := newClient(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		status := "pending"
		if n >= 3 {
			status = "done"
		}
		json.NewEncoder(w).Encode(videos.VideoResult{
			Status:   status,
			Progress: int(n * 33),
		})
	})
	defer srv.Close()

	result, err := c.Wait(context.Background(), "req-456", time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "done" {
		t.Errorf("Status = %q, want done", result.Status)
	}
	if calls.Load() < 3 {
		t.Errorf("expected at least 3 poll calls, got %d", calls.Load())
	}
}

func TestWait_failedWithError(t *testing.T) {
	srv, c := newClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(videos.VideoResult{
			Status: "failed",
			Error:  &videos.VideoError{Code: "content_policy", Message: "violates policy"},
		})
	})
	defer srv.Close()

	_, err := c.Wait(context.Background(), "req-789", time.Millisecond)
	if err == nil {
		t.Fatal("expected error on failed status")
	}
	errStr := err.Error()
	if errStr == "" {
		t.Error("error message is empty")
	}
	// Should include the code and message from VideoError.
	if !strings.Contains(errStr, "content_policy") || !strings.Contains(errStr, "violates policy") {
		t.Errorf("error %q should mention code and message", errStr)
	}
}

func TestWait_failedWithoutError(t *testing.T) {
	srv, c := newClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(videos.VideoResult{Status: "failed"})
	})
	defer srv.Close()

	_, err := c.Wait(context.Background(), "req-000", time.Millisecond)
	if err == nil {
		t.Fatal("expected error on failed status")
	}
}

func TestWait_contextCancelled(t *testing.T) {
	srv, c := newClient(func(w http.ResponseWriter, r *http.Request) {
		// Always pending, forces context cancellation path.
		json.NewEncoder(w).Encode(videos.VideoResult{Status: "pending"})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := c.Wait(ctx, "req-ctx", time.Millisecond)
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestWait_zeroIntervalDefaulted(t *testing.T) {
	// Passing pollInterval=0 should not panic; it uses the default.
	// Use a done response so Wait returns quickly.
	srv, c := newClient(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(videos.VideoResult{Status: "done"})
	})
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := c.Wait(ctx, "req-zero", 0)
	if err != nil {
		t.Fatal(err)
	}
}
