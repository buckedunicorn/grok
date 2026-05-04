package transport_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func newLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestTransport_Logger_successLogged(t *testing.T) {
	var buf bytes.Buffer
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{}"))
	})
	defer srv.Close()
	tr.Logger = newLogger(&buf)

	if err := tr.Do(context.Background(), "GET", "/test", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "request complete") {
		t.Errorf("expected 'request complete' in log output, got:\n%s", out)
	}
	if !strings.Contains(out, "/test") {
		t.Errorf("expected path in log output, got:\n%s", out)
	}
}

func TestTransport_Logger_errorLogged(t *testing.T) {
	var buf bytes.Buffer
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"code":"bad","error":"bad request"}`))
	})
	defer srv.Close()
	tr.Logger = newLogger(&buf)

	tr.Do(context.Background(), "GET", "/fail", nil, nil) //nolint:errcheck

	out := buf.String()
	if !strings.Contains(out, "request failed") {
		t.Errorf("expected 'request failed' in log output, got:\n%s", out)
	}
}

func TestTransport_Logger_retryLogged(t *testing.T) {
	var buf bytes.Buffer
	calls := 0
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			w.Write([]byte(`{"code":"unavailable","error":"down"}`))
			return
		}
		w.Write([]byte("{}"))
	})
	defer srv.Close()
	tr.MaxRetries = 1
	tr.Logger = newLogger(&buf)

	tr.Do(context.Background(), "GET", "/retry", nil, nil) //nolint:errcheck

	out := buf.String()
	if !strings.Contains(out, "retrying") {
		t.Errorf("expected 'retrying' in log output, got:\n%s", out)
	}
}
