package transport_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/internal/apierr"
	"github.com/buckedunicorn/grok/internal/transport"
)

func newTestTransport(handler http.HandlerFunc) (*httptest.Server, *transport.Transport) {
	srv := httptest.NewServer(handler)
	// httptest binds to loopback HTTP; use NewInsecure to opt out of
	// the production HTTPS-only enforcement.
	return srv, transport.NewInsecure("test-key", srv.URL, srv.Client())
}

func TestTransport_Do_success(t *testing.T) {
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"hello": "world"})
	})
	defer srv.Close()

	var out map[string]string
	if err := tr.Do(context.Background(), "GET", "/", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out["hello"] != "world" {
		t.Errorf("out[\"hello\"] = %q, want \"world\"", out["hello"])
	}
}

func TestTransport_Do_sendsAuthHeader(t *testing.T) {
	var gotAuth string
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte("{}"))
	})
	defer srv.Close()

	if err := tr.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
}

func TestTransport_Do_apiError_xAIFormat(t *testing.T) {
	// xAI returns {"code":"<short_identifier>","error":"<human_description>"}
	// at the top level. Code is a tight identifier (e.g. "INVALID_ARGUMENT"),
	// error is the human-readable explanation.
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"code":"INVALID_ARGUMENT","error":"Model grok-x does not support parameter reasoningEffort."}`))
	})
	defer srv.Close()

	err := tr.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *apierr.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierr.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Code != "INVALID_ARGUMENT" {
		t.Errorf("Code = %q, want %q", apiErr.Code, "INVALID_ARGUMENT")
	}
	if apiErr.Message != "Model grok-x does not support parameter reasoningEffort." {
		t.Errorf("Message = %q, want %q", apiErr.Message, "Model grok-x does not support parameter reasoningEffort.")
	}
}

func TestTransport_Do_apiError_unknownFormat(t *testing.T) {
	// Unknown format: Raw bytes populated; error string shows them.
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"unexpected":"shape"}`))
	})
	defer srv.Close()

	err := tr.Do(context.Background(), "GET", "/", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var apiErr *apierr.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierr.APIError, got %T", err)
	}
	if string(apiErr.Raw) != `{"unexpected":"shape"}` {
		t.Errorf("Raw = %q, want %q", apiErr.Raw, `{"unexpected":"shape"}`)
	}
	if !strings.Contains(err.Error(), `{"unexpected":"shape"}`) {
		t.Errorf("Error() = %q, should contain raw body", err.Error())
	}
}

func TestTransport_Do_nilOut(t *testing.T) {
	// out=nil should succeed on 2xx without decoding.
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	defer srv.Close()

	if err := tr.Do(context.Background(), "DELETE", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestTransport_WithConvID(t *testing.T) {
	var gotHeader string
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-grok-conv-id")
		w.Write([]byte("{}"))
	})
	defer srv.Close()

	tr2 := tr.WithConvID("conv-abc")
	if err := tr2.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotHeader != "conv-abc" {
		t.Errorf("x-grok-conv-id = %q, want %q", gotHeader, "conv-abc")
	}
}

func TestTransport_WithConvID_doesNotMutateOriginal(t *testing.T) {
	var gotHeader string
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("x-grok-conv-id")
		w.Write([]byte("{}"))
	})
	defer srv.Close()

	_ = tr.WithConvID("conv-xyz")
	if err := tr.Do(context.Background(), "GET", "/", nil, nil); err != nil {
		t.Fatal(err)
	}
	if gotHeader != "" {
		t.Errorf("original transport should not send x-grok-conv-id, got %q", gotHeader)
	}
}

func TestTransport_DoRaw_success(t *testing.T) {
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("audio bytes"))
	})
	defer srv.Close()

	raw, err := tr.DoRaw(context.Background(), "GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "audio bytes" {
		t.Errorf("DoRaw = %q, want %q", raw, "audio bytes")
	}
}

func TestTransport_DoRaw_error(t *testing.T) {
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"code":"not found","error":"not found"}`))
	})
	defer srv.Close()

	_, err := tr.DoRaw(context.Background(), "GET", "/", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *apierr.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierr.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
}

func TestTransport_DoMultipart_success(t *testing.T) {
	var gotContentType, gotField string
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_ = r.ParseMultipartForm(1 << 20)
		gotField = r.FormValue("model")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "f-123"})
	})
	defer srv.Close()

	fields := map[string]string{"model": "whisper"}
	var out map[string]string
	if err := tr.DoMultipart(context.Background(), "/", fields, "file", "audio.wav", strings.NewReader("bytes"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("Content-Type = %q, want multipart/form-data", gotContentType)
	}
	if gotField != "whisper" {
		t.Errorf("form field model = %q, want whisper", gotField)
	}
	if out["id"] != "f-123" {
		t.Errorf("out[\"id\"] = %q, want f-123", out["id"])
	}
}

func TestTransport_DoMultipart_returnsAPIError(t *testing.T) {
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"code":"bad request","error":"Bad Request"}`))
	})
	defer srv.Close()

	err := tr.DoMultipart(context.Background(), "/", nil, "file", "f.wav", strings.NewReader("x"), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *apierr.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierr.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
}

func TestSSEStream_Next_parsesEvents(t *testing.T) {
	body := "data: {\"id\":\"1\"}\n\ndata: {\"id\":\"2\"}\n\ndata: [DONE]\n\n"
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(body))
	})
	defer srv.Close()

	stream, err := tr.Stream(context.Background(), "POST", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() #1 error: %v", err)
	}
	if string(chunk) != `{"id":"1"}` {
		t.Errorf("chunk #1 = %q, want %q", chunk, `{"id":"1"}`)
	}

	chunk, err = stream.Next()
	if err != nil {
		t.Fatalf("Next() #2 error: %v", err)
	}
	if string(chunk) != `{"id":"2"}` {
		t.Errorf("chunk #2 = %q, want %q", chunk, `{"id":"2"}`)
	}

	_, err = stream.Next()
	if err != io.EOF {
		t.Errorf("Next() after [DONE] = %v, want io.EOF", err)
	}
}

func TestSSEStream_Next_skipsNonDataLines(t *testing.T) {
	// SSE spec: lines without "data:" prefix (comments, event:, id:, retry:) are skipped.
	body := ": comment\nevent: message\ndata: {\"x\":1}\n\ndata: [DONE]\n\n"
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(body))
	})
	defer srv.Close()

	stream, err := tr.Stream(context.Background(), "POST", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	chunk, err := stream.Next()
	if err != nil {
		t.Fatalf("Next() error: %v", err)
	}
	if string(chunk) != `{"x":1}` {
		t.Errorf("chunk = %q, want %q", chunk, `{"x":1}`)
	}
}

func TestTransport_Stream_httpError(t *testing.T) {
	srv, tr := newTestTransport(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"code":"forbidden","error":"Forbidden"}`))
	})
	defer srv.Close()

	_, err := tr.Stream(context.Background(), "POST", "/", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *apierr.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *apierr.APIError, got %T", err)
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("StatusCode = %d, want 403", apiErr.StatusCode)
	}
}
