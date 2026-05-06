// Package transport is the shared HTTP layer used by every public
// sub-client (chat, responses, images, ...). It owns base-URL handling,
// authentication, retry, response-size limits, and the SSE stream
// reader. The package is internal so callers configure it indirectly via
// the grok.With* options on the root client.
package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/buckedunicorn/grok/internal/apierr"
	"github.com/buckedunicorn/grok/internal/multipart"
)

const (
	defaultBaseURL          = "https://api.x.ai"
	defaultRequestTimeout   = 5 * time.Minute
	defaultMaxResponseBytes = 64 * 1024 * 1024 // 64 MiB
)

// Transport handles all HTTP communication with the xAI API.
type Transport struct {
	Client     *http.Client
	BaseURL    string
	APIKey     string
	ConvID     string        // x-grok-conv-id for prompt caching
	MaxRetries int           // 0 = no retry (default)
	Logger     *slog.Logger  // nil = no logging
	sem        chan struct{} // nil = unlimited concurrency

	// bearer is "Bearer " + APIKey, precomputed in newWithOptions to
	// avoid the per-request string concatenation.
	bearer string

	// MaxResponseBytes caps the body size for non-streaming responses.
	// Zero uses the default (64 MiB). Negative disables. Streaming
	// helpers (Stream, DoStream) bypass this cap by design.
	MaxResponseBytes int64

	// AllowInsecureBaseURL permits http:// base URLs. By default
	// transport.New rejects non-HTTPS base URLs to prevent silent
	// downgrade of the API-key bearer token. Test/dev callers should
	// set this explicitly.
	AllowInsecureBaseURL bool

	// baseURLErr, when non-nil, causes every Do/Stream call to return
	// it immediately. Set by New when the base URL fails validation.
	baseURLErr error
}

// APIError is re-exported so sub-packages can use it without importing apierr directly.
type APIError = apierr.APIError

// New creates a Transport. If baseURL is empty, the default xAI base URL
// is used. If client is nil a default http.Client is built with a
// 5-minute timeout. The base URL must be https:// unless
// allowInsecureBase is true (use NewInsecure for that).
func New(apiKey, baseURL string, client *http.Client) *Transport {
	t := newTransport(apiKey, baseURL, client, false)
	// Validate the scheme. On failure the transport is still returned
	// so callers see the error on first Do (preserving the historical
	// signature) rather than a nil-pointer panic.
	if err := requireHTTPS(t.BaseURL); err != nil {
		t.baseURLErr = err
	}
	return t
}

// NewInsecure is like New but accepts http:// base URLs. Use only for
// tests or for trusted on-network mirrors that do not speak TLS.
func NewInsecure(apiKey, baseURL string, client *http.Client) *Transport {
	return newTransport(apiKey, baseURL, client, true)
}

// newTransport assembles a *Transport with default fallbacks. It cannot
// fail; scheme validation is the caller's responsibility.
func newTransport(apiKey, baseURL string, client *http.Client, allowInsecure bool) *Transport {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if client == nil {
		client = defaultHTTPClient()
	}
	return &Transport{
		Client:               client,
		BaseURL:              baseURL,
		APIKey:               apiKey,
		AllowInsecureBaseURL: allowInsecure,
		bearer:               "Bearer " + apiKey,
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultRequestTimeout}
}

func requireHTTPS(raw string) error {
	if raw == "" {
		return nil
	}
	switch {
	case strings.HasPrefix(raw, "https://"):
		return nil
	case strings.HasPrefix(raw, "http://"):
		return fmt.Errorf("grok: base URL must be https:// (got %q); use NewInsecure or grok.WithInsecureBaseURL to opt in", raw)
	default:
		return fmt.Errorf("grok: base URL must include scheme (got %q)", raw)
	}
}

// WithConvID returns a shallow copy of t with ConvID set.
func (t *Transport) WithConvID(id string) *Transport {
	t2 := *t
	t2.ConvID = id
	return &t2
}

// WithConcurrency returns a shallow copy of t with a semaphore limiting the
// number of in-flight HTTP calls to n. The semaphore is shared across copies
// of t that are derived via WithConvID, so the limit applies globally.
func (t *Transport) WithConcurrency(n int) *Transport {
	t2 := *t
	if n > 0 {
		t2.sem = make(chan struct{}, n)
	}
	return &t2
}

// Do performs a JSON request and decodes the response into out.
func (t *Transport) Do(ctx context.Context, method, path string, body, out any) error {
	if t.baseURLErr != nil {
		return t.baseURLErr
	}
	bodyBytes, contentType, err := marshalBody(body)
	if err != nil {
		return err
	}
	return t.retry(ctx, method, path, func() error {
		req, err := t.buildRequest(ctx, method, path, bodyBytes, contentType)
		if err != nil {
			return err
		}
		resp, err := t.httpDo(ctx, req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, err := t.readResponseBody(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return t.apiError(resp.StatusCode, raw)
		}
		if out != nil {
			return json.Unmarshal(raw, out)
		}
		return nil
	})
}

// readResponseBody reads up to MaxResponseBytes (or the default) plus
// one byte. If the cap is hit, the read is rejected to prevent a
// misbehaving server from exhausting client memory.
func (t *Transport) readResponseBody(body io.Reader) ([]byte, error) {
	maxBytes := t.MaxResponseBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxResponseBytes
	}
	if maxBytes < 0 {
		return io.ReadAll(body)
	}
	raw, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("grok: response body exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

// Stream performs a request expecting an SSE (text/event-stream) response.
func (t *Transport) Stream(ctx context.Context, method, path string, body any) (*SSEStream, error) {
	if t.baseURLErr != nil {
		return nil, t.baseURLErr
	}
	bodyBytes, contentType, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	var result *SSEStream
	err = t.retry(ctx, method, path, func() error {
		req, err := t.buildRequest(ctx, method, path, bodyBytes, contentType)
		if err != nil {
			return err
		}
		resp, err := t.httpDo(ctx, req)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, _ := t.readResponseBody(resp.Body)
			resp.Body.Close()
			return t.apiError(resp.StatusCode, raw)
		}
		// Raise the per-line cap from the bufio default 64 KiB so a
		// long completion delta does not error with bufio.ErrTooLong.
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
		result = &SSEStream{scanner: scanner, resp: resp}
		return nil
	})
	return result, err
}

// DoRaw performs a request and returns raw bytes (e.g. for audio).
func (t *Transport) DoRaw(ctx context.Context, method, path string, body any) ([]byte, error) {
	if t.baseURLErr != nil {
		return nil, t.baseURLErr
	}
	bodyBytes, contentType, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	var result []byte
	err = t.retry(ctx, method, path, func() error {
		req, err := t.buildRequest(ctx, method, path, bodyBytes, contentType)
		if err != nil {
			return err
		}
		resp, err := t.httpDo(ctx, req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, err := t.readResponseBody(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return t.apiError(resp.StatusCode, raw)
		}
		result = raw
		return nil
	})
	return result, err
}

// DoStream performs a request and returns the response body as a ReadCloser.
// The caller must close the returned body. The MaxResponseBytes cap does
// not apply to DoStream; the caller controls how much to read.
func (t *Transport) DoStream(ctx context.Context, method, path string, body any) (io.ReadCloser, error) {
	if t.baseURLErr != nil {
		return nil, t.baseURLErr
	}
	bodyBytes, contentType, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	var result io.ReadCloser
	err = t.retry(ctx, method, path, func() error {
		req, err := t.buildRequest(ctx, method, path, bodyBytes, contentType)
		if err != nil {
			return err
		}
		resp, err := t.httpDo(ctx, req)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, _ := t.readResponseBody(resp.Body)
			resp.Body.Close()
			return t.apiError(resp.StatusCode, raw)
		}
		result = resp.Body
		return nil
	})
	return result, err
}

// DoMultipart sends a multipart/form-data POST and decodes the JSON response into out.
// fields are optional string key-value pairs. Pass a non-nil file with fileField/filename
// to include a file part; pass nil file for field-only forms (e.g. STT with a URL).
func (t *Transport) DoMultipart(ctx context.Context, path string, fields map[string]string, fileField, filename string, file io.Reader, out any) error {
	if t.baseURLErr != nil {
		return t.baseURLErr
	}
	// Build the full multipart body once; bytes.NewReader makes it re-readable for retries.
	buf, contentType, err := multipart.Build(fields, fileField, filename, file)
	if err != nil {
		return err
	}
	bodyBytes := buf.Bytes()
	return t.retry(ctx, "POST", path, func() error {
		req, err := t.buildRequest(ctx, "POST", path, bodyBytes, contentType)
		if err != nil {
			return err
		}
		resp, err := t.httpDo(ctx, req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, err := t.readResponseBody(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return t.apiError(resp.StatusCode, raw)
		}
		if out != nil {
			return json.Unmarshal(raw, out)
		}
		return nil
	})
}

// httpDo executes req, acquiring the concurrency semaphore if configured.
func (t *Transport) httpDo(ctx context.Context, req *http.Request) (*http.Response, error) {
	if t.sem != nil {
		select {
		case t.sem <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		defer func() { <-t.sem }()
	}
	return t.Client.Do(req)
}

// retry calls fn up to MaxRetries+1 times, waiting with exponential backoff between
// attempts. It stops early if the error is not retryable or the context is cancelled.
func (t *Transport) retry(ctx context.Context, method, path string, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		if attempt > 0 {
			if !isRetryable(lastErr) {
				return lastErr
			}
			wait := t.backoff(attempt)
			if t.Logger != nil {
				t.Logger.WarnContext(ctx, "grok: retrying request",
					slog.String("method", method),
					slog.String("path", path),
					slog.Int("attempt", attempt),
					slog.Duration("backoff", wait),
					slog.String("error", lastErr.Error()),
				)
			}
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		start := time.Now()
		lastErr = fn()
		elapsed := time.Since(start)

		if t.Logger != nil {
			switch {
			case lastErr == nil:
				t.Logger.InfoContext(ctx, "grok: request complete",
					slog.String("method", method),
					slog.String("path", path),
					slog.Duration("duration", elapsed),
				)
			case attempt == 0 || !isRetryable(lastErr):
				t.Logger.ErrorContext(ctx, "grok: request failed",
					slog.String("method", method),
					slog.String("path", path),
					slog.Duration("duration", elapsed),
					slog.String("error", lastErr.Error()),
				)
			default:
				// Mid-retry failure with a retryable error. The next
				// loop iteration will log "retrying request" before
				// the next attempt; this Debug entry preserves the
				// per-attempt failure so an operator can count
				// transient errors absorbed by the retry layer
				//.
				t.Logger.DebugContext(ctx, "grok: retryable request error",
					slog.String("method", method),
					slog.String("path", path),
					slog.Int("attempt", attempt),
					slog.Duration("duration", elapsed),
					slog.String("error", lastErr.Error()),
				)
			}
		}

		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// backoff returns the wait duration for the given attempt number (1-indexed).
// It uses exponential backoff capped at 30 s, plus up to 50% random jitter.
func (t *Transport) backoff(attempt int) time.Duration {
	const base = 500 * time.Millisecond
	const maxWait = 30 * time.Second
	exp := min(attempt-1, 10)
	wait := min(base*(1<<uint(exp)), maxWait)
	jitter := time.Duration(rand.Int64N(int64(wait) / 2))
	return wait + jitter
}

// isRetryable reports whether err warrants a retry attempt.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *apierr.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 429, 500, 502, 503, 504:
			return true
		}
		return false // other 4xx/5xx are permanent
	}
	return true // network / transport errors are transient
}

// bufferPool recycles *bytes.Buffer instances used as encoding scratch.
// json.Marshal allocates a fresh slice every call; reusing a buffer
// across requests cuts the steady-state allocation rate for the
// transport hot path roughly in half.
var bufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func getBuffer() *bytes.Buffer  { return bufferPool.Get().(*bytes.Buffer) }
func putBuffer(b *bytes.Buffer) { b.Reset(); bufferPool.Put(b) }

func marshalBody(body any) ([]byte, string, error) {
	if body == nil {
		return nil, "", nil
	}
	buf := getBuffer()
	defer putBuffer(buf)
	enc := json.NewEncoder(buf)
	if err := enc.Encode(body); err != nil {
		return nil, "", err
	}
	// json.Encoder appends a trailing newline; strip it so the wire
	// payload matches what json.Marshal would have produced.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	// Detach from the pooled buffer before returning.
	cp := make([]byte, len(out))
	copy(cp, out)
	return cp, "application/json", nil
}

func (t *Transport) buildRequest(ctx context.Context, method, path string, bodyBytes []byte, contentType string) (*http.Request, error) {
	var r io.Reader
	if bodyBytes != nil {
		r = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.BaseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", t.bearer)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if t.ConvID != "" {
		req.Header.Set("x-grok-conv-id", t.ConvID)
	}
	return req, nil
}

// apiErrorRawCap caps the bytes preserved in APIError.Raw so that a
// large server error body cannot blow up structured logs that include
// the error chain.
const apiErrorRawCap = 4096

const apiErrorTruncMarker = "... [truncated]"

func (t *Transport) apiError(status int, raw []byte) error {
	// xAI returns {"code": "...", "error": "..."} at the top level, where
	// "code" is a short identifier (e.g. "rate_limited") and "error" is a
	// human-readable description.
	var e struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	preserved := raw
	if len(raw) > apiErrorRawCap {
		// Single allocation with the right final capacity.
		preserved = make([]byte, 0, apiErrorRawCap+len(apiErrorTruncMarker))
		preserved = append(preserved, raw[:apiErrorRawCap]...)
		preserved = append(preserved, apiErrorTruncMarker...)
	}
	return &apierr.APIError{
		StatusCode: status,
		Code:       e.Code,
		Message:    e.Error,
		Raw:        preserved,
	}
}

// SSEStream reads server-sent events from an HTTP response.
type SSEStream struct {
	scanner *bufio.Scanner
	resp    *http.Response
}

// Next returns the raw JSON bytes for the next SSE data event.
// Returns io.EOF when the stream is done ([DONE] sentinel received).
//
// The returned []byte aliases the scanner's internal buffer and is
// only valid until the next call to Next. Callers that need to retain
// the bytes must copy them. In-package callers (chat.Stream.Next,
// responses.Stream.Next) immediately json.Unmarshal the slice, which
// is safe.
var (
	dataPrefix = []byte("data:")
	doneToken  = []byte("[DONE]")
)

func (s *SSEStream) Next() ([]byte, error) {
	for s.scanner.Scan() {
		line := s.scanner.Bytes()
		if !bytes.HasPrefix(line, dataPrefix) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, dataPrefix))
		if bytes.Equal(data, doneToken) {
			return nil, io.EOF
		}
		return data, nil
	}
	if err := s.scanner.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// Close releases the underlying HTTP response body.
func (s *SSEStream) Close() error {
	return s.resp.Body.Close()
}
