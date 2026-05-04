// Package images provides the /v1/images/generations and /v1/images/edits endpoints.
package images

import (
	"context"

	"github.com/buckedunicorn/grok/internal/transport"
)

// Client wraps the image generation endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates an images Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// GenerateRequest is the body for POST /v1/images/generations.
type GenerateRequest struct {
	Model          string `json:"model,omitempty"`
	Prompt         string `json:"prompt"`
	N              *int   `json:"n,omitempty"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`    // "1:1" | "16:9" | etc.
	Quality        string `json:"quality,omitempty"`         // "low" | "medium" | "high"
	Resolution     string `json:"resolution,omitempty"`      // "1k" | "2k"
	ResponseFormat string `json:"response_format,omitempty"` // "url" | "b64_json"
	User           string `json:"user,omitempty"`
}

// EditRequest is the body for POST /v1/images/edits.
type EditRequest struct {
	Model          string        `json:"model,omitempty"`
	Prompt         string        `json:"prompt"`
	Image          *ImageSource  `json:"image,omitempty"`
	Images         []ImageSource `json:"images,omitempty"` // multi-reference; mutually exclusive with Image
	Mask           *ImageSource  `json:"mask,omitempty"`
	N              *int          `json:"n,omitempty"`
	AspectRatio    string        `json:"aspect_ratio,omitempty"`
	Quality        string        `json:"quality,omitempty"`
	Resolution     string        `json:"resolution,omitempty"`
	ResponseFormat string        `json:"response_format,omitempty"`
	User           string        `json:"user,omitempty"`
}

type ImageSource struct {
	URL string `json:"url"`
}

// ImageResponse is returned by both generation and edit endpoints.
type ImageResponse struct {
	Data  []ImageData `json:"data"`
	Usage *ImageUsage `json:"usage"`
}

type ImageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	RevisedPrompt string `json:"revised_prompt"`
}

type ImageUsage struct {
	CostInUSDTicks int64 `json:"cost_in_usd_ticks"`
}

// Generate calls POST /v1/images/generations and returns the generated images.
func (c *Client) Generate(ctx context.Context, req *GenerateRequest) (*ImageResponse, error) {
	var out ImageResponse
	if err := c.t.Do(ctx, "POST", "/v1/images/generations", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Edit calls POST /v1/images/edits and returns the edited images.
func (c *Client) Edit(ctx context.Context, req *EditRequest) (*ImageResponse, error) {
	var out ImageResponse
	if err := c.t.Do(ctx, "POST", "/v1/images/edits", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
