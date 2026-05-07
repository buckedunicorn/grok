// Package models provides access to the xAI model listing endpoints.
//
// Four list endpoints are supported — [Client.List] (all models),
// [Client.ListLanguage] (language models with full pricing info),
// [Client.ListImageGeneration], and [Client.ListVideoGeneration] — plus
// [Client.Get] and [Client.GetLanguage] for fetching a single model by ID.
package models

import (
	"context"
	"net/url"

	"github.com/buckedunicorn/grok/internal/idvalidate"
	"github.com/buckedunicorn/grok/internal/transport"
)

// Client wraps the models endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates a models Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// Model is the minimal model representation from GET /v1/models.
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// LanguageModel is the full model representation from GET /v1/language-models.
type LanguageModel struct {
	ID                         string   `json:"id"`
	Object                     string   `json:"object"`
	Created                    int64    `json:"created"`
	OwnedBy                    string   `json:"owned_by"`
	Version                    string   `json:"version"`
	Fingerprint                string   `json:"fingerprint"`
	InputModalities            []string `json:"input_modalities"`
	OutputModalities           []string `json:"output_modalities"`
	Aliases                    []string `json:"aliases"`
	PromptTextTokenPrice       int64    `json:"prompt_text_token_price"`
	CachedPromptTextTokenPrice int64    `json:"cached_prompt_text_token_price"`
	PromptImageTokenPrice      int64    `json:"prompt_image_token_price"`
	CompletionTextTokenPrice   int64    `json:"completion_text_token_price"`
	SearchPrice                int64    `json:"search_price"`
}

// ImageModel is the full model representation from GET /v1/image-generation-models.
type ImageModel struct {
	ID               string   `json:"id"`
	Object           string   `json:"object"`
	Created          int64    `json:"created"`
	OwnedBy          string   `json:"owned_by"`
	Version          string   `json:"version"`
	Fingerprint      string   `json:"fingerprint"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Aliases          []string `json:"aliases"`
	ImagePrice       int64    `json:"image_price"`
	MaxPromptLength  int      `json:"max_prompt_length"`
}

// VideoModel is the full model representation from GET /v1/video-generation-models.
type VideoModel struct {
	ID               string   `json:"id"`
	Object           string   `json:"object"`
	Created          int64    `json:"created"`
	OwnedBy          string   `json:"owned_by"`
	Version          string   `json:"version"`
	Fingerprint      string   `json:"fingerprint"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	Aliases          []string `json:"aliases"`
}

// List returns all models available to the authenticating key (minimal info).
func (c *Client) List(ctx context.Context) ([]Model, error) {
	var out struct {
		Data   []Model `json:"data"`
		Object string  `json:"object"`
	}
	if err := c.t.Do(ctx, "GET", "/v1/models", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Get returns minimal info for a single model.
func (c *Client) Get(ctx context.Context, modelID string) (*Model, error) {
	if err := idvalidate.OpaqueID("grok/models", "modelID", modelID); err != nil {
		return nil, err
	}
	var out Model
	if err := c.t.Do(ctx, "GET", "/v1/models/"+url.PathEscape(modelID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListLanguage returns all chat/vision language models with full pricing info.
func (c *Client) ListLanguage(ctx context.Context) ([]LanguageModel, error) {
	var out struct {
		Models []LanguageModel `json:"models"`
	}
	if err := c.t.Do(ctx, "GET", "/v1/language-models", nil, &out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

// GetLanguage returns full info for a single language model.
func (c *Client) GetLanguage(ctx context.Context, modelID string) (*LanguageModel, error) {
	if err := idvalidate.OpaqueID("grok/models", "modelID", modelID); err != nil {
		return nil, err
	}
	var out LanguageModel
	if err := c.t.Do(ctx, "GET", "/v1/language-models/"+url.PathEscape(modelID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListImageGeneration returns all image generation models with full info.
func (c *Client) ListImageGeneration(ctx context.Context) ([]ImageModel, error) {
	var out struct {
		Models []ImageModel `json:"models"`
	}
	if err := c.t.Do(ctx, "GET", "/v1/image-generation-models", nil, &out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

// GetImageGeneration returns full info for a single image generation model.
func (c *Client) GetImageGeneration(ctx context.Context, modelID string) (*ImageModel, error) {
	if err := idvalidate.OpaqueID("grok/models", "modelID", modelID); err != nil {
		return nil, err
	}
	var out ImageModel
	if err := c.t.Do(ctx, "GET", "/v1/image-generation-models/"+url.PathEscape(modelID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListVideoGeneration returns all video generation models with full info.
func (c *Client) ListVideoGeneration(ctx context.Context) ([]VideoModel, error) {
	var out struct {
		Models []VideoModel `json:"models"`
	}
	if err := c.t.Do(ctx, "GET", "/v1/video-generation-models", nil, &out); err != nil {
		return nil, err
	}
	return out.Models, nil
}

// GetVideoGeneration returns full info for a single video generation model.
func (c *Client) GetVideoGeneration(ctx context.Context, modelID string) (*VideoModel, error) {
	if err := idvalidate.OpaqueID("grok/models", "modelID", modelID); err != nil {
		return nil, err
	}
	var out VideoModel
	if err := c.t.Do(ctx, "GET", "/v1/video-generation-models/"+url.PathEscape(modelID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
