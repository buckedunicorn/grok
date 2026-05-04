// Package batches provides the /v1/batches endpoints for async bulk inference at 50% cost.
package batches

import (
	"context"
	"fmt"
	"iter"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buckedunicorn/grok/internal/idvalidate"
	"github.com/buckedunicorn/grok/internal/transport"
)

// Client wraps the batch endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates a batches Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// Batch represents a batch job.
type Batch struct {
	BatchID            string     `json:"batch_id"`
	Name               string     `json:"name"`
	CreateTime         string     `json:"create_time"`
	ExpireTime         *string    `json:"expire_time"`
	CancelTime         *string    `json:"cancel_time"`
	CancelByXAIMessage *string    `json:"cancel_by_xai_message"`
	CreateAPIKeyID     string     `json:"create_api_key_id"`
	State              BatchState `json:"state"`
}

type BatchState struct {
	NumRequests  int `json:"num_requests"`
	NumPending   int `json:"num_pending"`
	NumSuccess   int `json:"num_success"`
	NumError     int `json:"num_error"`
	NumCancelled int `json:"num_cancelled"`
}

// BatchList is the paginated response from GET /v1/batches.
type BatchList struct {
	Batches         []Batch `json:"batches"`
	PaginationToken *string `json:"pagination_token"`
}

// HasMore reports whether there are more pages.
func (l *BatchList) HasMore() bool { return l.PaginationToken != nil && *l.PaginationToken != "" }

// BatchRequest wraps a single request to be added to a batch.
type BatchRequest struct {
	BatchRequestID string              `json:"batch_request_id,omitempty"`
	BatchRequest   BatchRequestPayload `json:"batch_request"`
}

// BatchRequestPayload holds the actual chat completion request.
type BatchRequestPayload struct {
	ChatGetCompletion any `json:"chat_get_completion"`
}

// RequestMetadata describes a single request within a batch.
type RequestMetadata struct {
	BatchRequestID string  `json:"batch_request_id"`
	Endpoint       string  `json:"endpoint"`
	Model          string  `json:"model"`
	State          string  `json:"state"` // "unknown"|"pending"|"succeeded"|"cancelled"|"failed"
	CreateTime     string  `json:"create_time"`
	FinishTime     *string `json:"finish_time"`
}

// RequestList is the paginated response from GET /v1/batches/{id}/requests.
type RequestList struct {
	BatchRequestMetadata []RequestMetadata `json:"batch_request_metadata"`
	PaginationToken      *string           `json:"pagination_token"`
}

// BatchResult holds the result for a single batch request.
type BatchResult struct {
	BatchRequestID string      `json:"batch_request_id"`
	BatchResult    ResultUnion `json:"batch_result"`
}

type ResultUnion struct {
	Response *ResultResponse `json:"response,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type ResultResponse struct {
	ChatGetCompletion any `json:"chat_get_completion"`
}

// ResultList is the paginated response from GET /v1/batches/{id}/results.
type ResultList struct {
	Results         []BatchResult `json:"results"`
	PaginationToken *string       `json:"pagination_token"`
}

// ListOptions configures paginated list requests.
type ListOptions struct {
	Limit           int
	PaginationToken string
}

// All returns an iterator over every batch, fetching pages automatically.
func (c *Client) All(ctx context.Context, opts *ListOptions) iter.Seq2[*Batch, error] {
	return func(yield func(*Batch, error) bool) {
		cur := opts
		for {
			page, err := c.List(ctx, cur)
			if err != nil {
				yield(nil, err)
				return
			}
			for i := range page.Batches {
				if !yield(&page.Batches[i], nil) {
					return
				}
			}
			if page.PaginationToken == nil || *page.PaginationToken == "" {
				return
			}
			next := &ListOptions{}
			if cur != nil {
				*next = *cur
			}
			next.PaginationToken = *page.PaginationToken
			cur = next
		}
	}
}

// AllRequests returns an iterator over all request metadata for a batch.
func (c *Client) AllRequests(ctx context.Context, batchID string, opts *ListOptions) iter.Seq2[*RequestMetadata, error] {
	return func(yield func(*RequestMetadata, error) bool) {
		cur := opts
		for {
			page, err := c.ListRequests(ctx, batchID, cur)
			if err != nil {
				yield(nil, err)
				return
			}
			for i := range page.BatchRequestMetadata {
				if !yield(&page.BatchRequestMetadata[i], nil) {
					return
				}
			}
			if page.PaginationToken == nil || *page.PaginationToken == "" {
				return
			}
			next := &ListOptions{}
			if cur != nil {
				*next = *cur
			}
			next.PaginationToken = *page.PaginationToken
			cur = next
		}
	}
}

// AllResults returns an iterator over all processing results for a batch.
func (c *Client) AllResults(ctx context.Context, batchID string, opts *ListOptions) iter.Seq2[*BatchResult, error] {
	return func(yield func(*BatchResult, error) bool) {
		cur := opts
		for {
			page, err := c.GetResults(ctx, batchID, cur)
			if err != nil {
				yield(nil, err)
				return
			}
			for i := range page.Results {
				if !yield(&page.Results[i], nil) {
					return
				}
			}
			if page.PaginationToken == nil || *page.PaginationToken == "" {
				return
			}
			next := &ListOptions{}
			if cur != nil {
				*next = *cur
			}
			next.PaginationToken = *page.PaginationToken
			cur = next
		}
	}
}

// Wait polls until all requests in the batch are processed (State.NumPending == 0),
// or the context is cancelled. pollInterval of 0 uses the default of 10 seconds.
func (c *Client) Wait(ctx context.Context, batchID string, pollInterval time.Duration) (*Batch, error) {
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	for {
		b, err := c.Get(ctx, batchID)
		if err != nil {
			return nil, err
		}
		if b.State.NumPending == 0 {
			return b, nil
		}
		t := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

// Create creates a new empty batch with the given name.
func (c *Client) Create(ctx context.Context, name string) (*Batch, error) {
	var out Batch
	if err := c.t.Do(ctx, "POST", "/v1/batches", map[string]string{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List returns all batches (paginated).
func (c *Client) List(ctx context.Context, opts *ListOptions) (*BatchList, error) {
	path := paginatedPath("/v1/batches", opts)
	var out BatchList
	if err := c.t.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns a single batch by ID.
func (c *Client) Get(ctx context.Context, batchID string) (*Batch, error) {
	if err := validateBatchID(batchID); err != nil {
		return nil, err
	}
	var out Batch
	if err := c.t.Do(ctx, "GET", "/v1/batches/"+url.PathEscape(batchID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AddRequests appends requests to an existing batch.
func (c *Client) AddRequests(ctx context.Context, batchID string, reqs []BatchRequest) error {
	if err := validateBatchID(batchID); err != nil {
		return err
	}
	body := map[string]any{"batch_requests": reqs}
	return c.t.Do(ctx, "POST", "/v1/batches/"+url.PathEscape(batchID)+"/requests", body, nil)
}

// ListRequests returns request metadata for a batch (paginated).
func (c *Client) ListRequests(ctx context.Context, batchID string, opts *ListOptions) (*RequestList, error) {
	if err := validateBatchID(batchID); err != nil {
		return nil, err
	}
	path := paginatedPath("/v1/batches/"+url.PathEscape(batchID)+"/requests", opts)
	var out RequestList
	if err := c.t.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetResults returns processing results for a batch (paginated).
func (c *Client) GetResults(ctx context.Context, batchID string, opts *ListOptions) (*ResultList, error) {
	if err := validateBatchID(batchID); err != nil {
		return nil, err
	}
	path := paginatedPath("/v1/batches/"+url.PathEscape(batchID)+"/results", opts)
	var out ResultList
	if err := c.t.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Cancel cancels all pending requests in a batch. xAI uses the
// colon-suffix form /v1/batches/{id}:cancel; the colon is not part of
// the ID and must NOT be percent-escaped.
func (c *Client) Cancel(ctx context.Context, batchID string) (*Batch, error) {
	if err := validateBatchID(batchID); err != nil {
		return nil, err
	}
	var out Batch
	if err := c.t.Do(ctx, "POST", "/v1/batches/"+url.PathEscape(batchID)+":cancel", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// validateBatchID rejects path separators and control characters in
// caller-supplied batch IDs. The colon is permitted by
// idvalidate (xAI IDs do not use it), but Cancel uses ":cancel" as a
// suffix; if a future ID format ever included a colon, the URL would
// become ambiguous. We reject ':' here as a defense in depth.
func validateBatchID(id string) error {
	if err := idvalidate.OpaqueID("grok/batches", "batchID", id); err != nil {
		return err
	}
	if strings.ContainsRune(id, ':') {
		return fmt.Errorf("grok/batches: batchID must not contain ':': %q", id)
	}
	return nil
}

func paginatedPath(base string, opts *ListOptions) string {
	if opts == nil {
		return base
	}
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.PaginationToken != "" {
		q.Set("pagination_token", opts.PaginationToken)
	}
	if len(q) == 0 {
		return base
	}
	return base + "?" + q.Encode()
}
