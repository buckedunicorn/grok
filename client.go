// Package grok is an idiomatic Go client SDK for the xAI Grok API.
//
// Create a client with [New], then access each API surface through its
// sub-client:
//
//	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
//	resp, err := client.Chat.Create(ctx, &chat.CreateRequest{...})
//
// API surfaces exposed on [Client]:
//   - [Client.Chat]      — chat completions, streaming, tool use ([chat.Client])
//   - [Client.Responses] — stateful Responses API ([responses.Client])
//   - [Client.Images]    — image generation and editing ([images.Client])
//   - [Client.Videos]    — async video generation ([videos.Client])
//   - [Client.Voice]     — TTS, STT, and realtime voice ([voice.Client])
//   - [Client.Models]    — model listing ([models.Client])
//   - [Client.Files]     — file upload and download ([files.Client])
//   - [Client.Batches]   — async bulk inference ([batches.Client])
//   - [Client.NewGRPC]   — gRPC transport alternative
package grok

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/buckedunicorn/grok/batches"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/files"
	xgrpc "github.com/buckedunicorn/grok/grpc"
	"github.com/buckedunicorn/grok/images"
	"github.com/buckedunicorn/grok/internal/transport"
	"github.com/buckedunicorn/grok/models"
	"github.com/buckedunicorn/grok/responses"
	"github.com/buckedunicorn/grok/videos"
	"github.com/buckedunicorn/grok/voice"
	"google.golang.org/grpc"
)

// Client is the top-level SDK entry point. Use New() to create one.
type Client struct {
	Chat      *chat.Client
	Responses *responses.Client
	Images    *images.Client
	Videos    *videos.Client
	Voice     *voice.Client
	Models    *models.Client
	Files     *files.Client
	Batches   *batches.Client
	t         *transport.Transport
}

type config struct {
	apiKey            string
	baseURL           string
	httpClient        *http.Client
	convID            string
	maxRetries        int
	logger            *slog.Logger
	concurrency       int
	allowInsecureBase bool
}

// Option configures the Client.
type Option func(*config)

// WithAPIKey sets the xAI API key. Defaults to the XAI_API_KEY environment variable.
func WithAPIKey(key string) Option {
	return func(c *config) { c.apiKey = key }
}

// WithBaseURL overrides the API base URL (default: https://api.x.ai).
// The URL must use the https scheme; non-HTTPS values are rejected to
// prevent silent downgrade of the API key. For testing or trusted
// internal mirrors that do not speak TLS, combine with
// WithInsecureBaseURL.
func WithBaseURL(url string) Option {
	return func(c *config) { c.baseURL = url }
}

// WithInsecureBaseURL permits non-HTTPS base URLs paired with
// WithBaseURL. Use only for local tests or trusted on-network
// mirrors. The bearer token is sent in cleartext over plain HTTP.
func WithInsecureBaseURL() Option {
	return func(c *config) { c.allowInsecureBase = true }
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) { c.httpClient = hc }
}

// WithConvID sets the x-grok-conv-id header on all requests from this client,
// which maximises prompt-cache hit rates across consecutive turns.
func WithConvID(id string) Option {
	return func(c *config) { c.convID = id }
}

// WithMaxRetries configures automatic retry with exponential backoff for transient
// errors (429, 500, 502, 503, 504, and network errors). n=0 disables retries (default).
func WithMaxRetries(n int) Option {
	return func(c *config) { c.maxRetries = n }
}

// WithLogger attaches a slog.Logger that receives request/response events.
// Successful requests are logged at Info; retries at Warn; failures at Error.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithConcurrency limits the number of simultaneous in-flight HTTP requests to n.
// Requests beyond the limit block until a slot is free. 0 means unlimited (default).
func WithConcurrency(n int) Option {
	return func(c *config) { c.concurrency = n }
}

// New creates a Client. If no WithAPIKey option is provided, it reads XAI_API_KEY from env.
func New(opts ...Option) *Client {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.apiKey == "" {
		cfg.apiKey = os.Getenv("XAI_API_KEY")
	}
	var t *transport.Transport
	if cfg.allowInsecureBase {
		t = transport.NewInsecure(cfg.apiKey, cfg.baseURL, cfg.httpClient)
	} else {
		t = transport.New(cfg.apiKey, cfg.baseURL, cfg.httpClient)
	}
	t.MaxRetries = cfg.maxRetries
	t.Logger = cfg.logger
	if cfg.concurrency > 0 {
		t = t.WithConcurrency(cfg.concurrency)
	}
	if cfg.convID != "" {
		t = t.WithConvID(cfg.convID)
	}
	return &Client{
		Chat:      chat.NewClient(t),
		Responses: responses.NewClient(t),
		Images:    images.NewClient(t),
		Videos:    videos.NewClient(t),
		Voice:     voice.NewClient(t),
		Models:    models.NewClient(t),
		Files:     files.NewClient(t),
		Batches:   batches.NewClient(t),
		t:         t,
	}
}

// WithConvID returns a new Client with the x-grok-conv-id header set for all requests.
// Use this to scope a conversation for prompt caching without recreating the full client.
func (c *Client) WithConvID(id string) *Client {
	t := c.t.WithConvID(id)
	return &Client{
		Chat:      chat.NewClient(t),
		Responses: responses.NewClient(t),
		Images:    images.NewClient(t),
		Videos:    videos.NewClient(t),
		Voice:     voice.NewClient(t),
		Models:    models.NewClient(t),
		Files:     files.NewClient(t),
		Batches:   batches.NewClient(t),
		t:         t,
	}
}

// NewGRPC creates a gRPC client for the xAI API at api.x.ai:443.
// The gRPC surface mirrors the REST inference endpoints; proto definitions
// are at https://github.com/xai-org/xai-proto.
// Additional dial options (e.g. interceptors) may be passed via opts.
func (c *Client) NewGRPC(opts ...grpc.DialOption) (*xgrpc.Client, error) {
	return xgrpc.New(c.t.APIKey, opts...)
}
