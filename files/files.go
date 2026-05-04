// Package files provides the /v1/files endpoints for managing uploaded files.
package files

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/buckedunicorn/grok/internal/idvalidate"
	"github.com/buckedunicorn/grok/internal/transport"
)

// Client wraps the files endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates a files Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// File represents a stored file.
type File struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt *int64 `json:"expires_at"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
}

// FileList is the paginated response from GET /v1/files.
type FileList struct {
	Data            []File  `json:"data"`
	PaginationToken *string `json:"pagination_token"`
}

// HasMore reports whether there are more pages to fetch.
func (l *FileList) HasMore() bool { return l.PaginationToken != nil && *l.PaginationToken != "" }

// ListOptions configures a paginated file list request.
type ListOptions struct {
	Limit           int
	Order           string // "asc" | "desc"
	SortBy          string // "created_at" | "filename" | "size"
	PaginationToken string
}

// List calls GET /v1/files (paginated).
func (c *Client) List(ctx context.Context, opts *ListOptions) (*FileList, error) {
	q := url.Values{}
	if opts != nil {
		if opts.Limit > 0 {
			q.Set("limit", strconv.Itoa(opts.Limit))
		}
		if opts.Order != "" {
			q.Set("order", opts.Order)
		}
		if opts.SortBy != "" {
			q.Set("sort_by", opts.SortBy)
		}
		if opts.PaginationToken != "" {
			q.Set("pagination_token", opts.PaginationToken)
		}
	}
	path := "/v1/files"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out FileList
	if err := c.t.Do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get retrieves metadata for a single file.
func (c *Client) Get(ctx context.Context, fileID string) (*File, error) {
	if err := validateOpaqueID("fileID", fileID); err != nil {
		return nil, err
	}
	var out File
	if err := c.t.Do(ctx, "GET", "/v1/files/"+url.PathEscape(fileID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// validateOpaqueID delegates to the shared idvalidate helper.
// See (security) and (deduplication).
func validateOpaqueID(field, id string) error {
	return idvalidate.OpaqueID("grok/files", field, id)
}

// All returns an iterator over every file, fetching pages automatically.
// The caller can break early; pagination stops immediately.
func (c *Client) All(ctx context.Context, opts *ListOptions) iter.Seq2[*File, error] {
	return func(yield func(*File, error) bool) {
		cur := opts
		for {
			page, err := c.List(ctx, cur)
			if err != nil {
				yield(nil, err)
				return
			}
			for i := range page.Data {
				if !yield(&page.Data[i], nil) {
					return
				}
			}
			if !page.HasMore() {
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

// Upload uploads a file via multipart/form-data POST /v1/files.
func (c *Client) Upload(ctx context.Context, filename string, r io.Reader) (*File, error) {
	var out File
	if err := c.t.DoMultipart(ctx, "/v1/files", nil, "file", filename, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Download returns the content of a file as a ReadCloser. Caller must close it.
func (c *Client) Download(ctx context.Context, fileID string) (io.ReadCloser, error) {
	if err := validateOpaqueID("fileID", fileID); err != nil {
		return nil, err
	}
	return c.t.DoStream(ctx, "GET", "/v1/files/"+url.PathEscape(fileID)+"/content", nil)
}

// Delete removes a file by ID.
func (c *Client) Delete(ctx context.Context, fileID string) error {
	if err := validateOpaqueID("fileID", fileID); err != nil {
		return err
	}
	return c.t.Do(ctx, "DELETE", "/v1/files/"+url.PathEscape(fileID), nil, nil)
}

// UploadPath opens the file at path and uploads it, using the base name as the filename.
func (c *Client) UploadPath(ctx context.Context, path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("grok/files: opening %s: %w", path, err)
	}
	defer f.Close()
	return c.Upload(ctx, filepath.Base(path), f)
}
