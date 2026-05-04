// Package videos provides the /v1/videos/* endpoints.
// Video generation is asynchronous, create returns a request_id; poll until status == "done".
package videos

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/buckedunicorn/grok/internal/idvalidate"
	"github.com/buckedunicorn/grok/internal/transport"
)

// Client wraps the video generation endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates a videos Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// GenerateRequest is the body for POST /v1/videos/generations.
type GenerateRequest struct {
	Model           string        `json:"model,omitempty"`
	Prompt          string        `json:"prompt,omitempty"`
	Image           *VideoSource  `json:"image,omitempty"`
	ReferenceImages []VideoSource `json:"reference_images,omitempty"`
	AspectRatio     string        `json:"aspect_ratio,omitempty"` // "1:1" | "16:9" | "9:16" | etc.
	Duration        *int          `json:"duration,omitempty"`     // seconds, 1–15
	Resolution      string        `json:"resolution,omitempty"`   // "480p" | "720p"
	User            string        `json:"user,omitempty"`
}

// EditRequest is the body for POST /v1/videos/edits.
type EditRequest struct {
	Model  string      `json:"model,omitempty"`
	Prompt string      `json:"prompt"`
	Video  VideoSource `json:"video"`
	User   string      `json:"user,omitempty"`
}

// ExtendRequest is the body for POST /v1/videos/extensions.
type ExtendRequest struct {
	Model    string      `json:"model,omitempty"`
	Prompt   string      `json:"prompt"`
	Video    VideoSource `json:"video"`
	Duration *int        `json:"duration,omitempty"` // seconds, 1–10
}

type VideoSource struct {
	URL string `json:"url"`
}

// asyncResponse is returned by create/edit/extend, just the request_id.
type asyncResponse struct {
	RequestID string `json:"request_id"`
}

// VideoResult is returned by GET /v1/videos/{request_id}.
type VideoResult struct {
	Status   string      `json:"status"` // "pending" | "done" | "failed"
	Progress int         `json:"progress"`
	Model    string      `json:"model"`
	Video    *VideoData  `json:"video"`
	Error    *VideoError `json:"error"`
	Usage    *VideoUsage `json:"usage"`
}

type VideoData struct {
	URL               string `json:"url"`
	Duration          int    `json:"duration"`
	RespectModeration bool   `json:"respect_moderation"`
}

type VideoError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type VideoUsage struct {
	CostInUSDTicks int64 `json:"cost_in_usd_ticks"`
}

// Generate submits a text-to-video (or image-to-video) request and returns the request_id.
func (c *Client) Generate(ctx context.Context, req *GenerateRequest) (string, error) {
	var out asyncResponse
	if err := c.t.Do(ctx, "POST", "/v1/videos/generations", req, &out); err != nil {
		return "", err
	}
	return out.RequestID, nil
}

// Edit submits a video edit request and returns the request_id.
func (c *Client) Edit(ctx context.Context, req *EditRequest) (string, error) {
	var out asyncResponse
	if err := c.t.Do(ctx, "POST", "/v1/videos/edits", req, &out); err != nil {
		return "", err
	}
	return out.RequestID, nil
}

// Extend submits a video extension request and returns the request_id.
func (c *Client) Extend(ctx context.Context, req *ExtendRequest) (string, error) {
	var out asyncResponse
	if err := c.t.Do(ctx, "POST", "/v1/videos/extensions", req, &out); err != nil {
		return "", err
	}
	return out.RequestID, nil
}

// GetResult polls GET /v1/videos/{request_id} once and returns the current status.
func (c *Client) GetResult(ctx context.Context, requestID string) (*VideoResult, error) {
	if err := validateOpaqueID("requestID", requestID); err != nil {
		return nil, err
	}
	var out VideoResult
	if err := c.t.Do(ctx, "GET", "/v1/videos/"+url.PathEscape(requestID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// validateOpaqueID delegates to the shared idvalidate helper.
func validateOpaqueID(field, id string) error {
	return idvalidate.OpaqueID("grok/videos", field, id)
}

// Wait polls GetResult at pollInterval until status is "done" or "failed",
// or the context is cancelled. Returns the final VideoResult on success.
func (c *Client) Wait(ctx context.Context, requestID string, pollInterval time.Duration) (*VideoResult, error) {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	for {
		result, err := c.GetResult(ctx, requestID)
		if err != nil {
			return nil, err
		}
		switch result.Status {
		case "done":
			return result, nil
		case "failed":
			if result.Error != nil {
				return nil, fmt.Errorf("video generation failed: %s: %s", result.Error.Code, result.Error.Message)
			}
			return nil, fmt.Errorf("video generation failed")
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
