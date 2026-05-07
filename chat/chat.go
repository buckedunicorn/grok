// Package chat provides the /v1/chat/completions endpoint (OpenAI-compatible).
//
// The main entry point is [Client], obtained via the root grok.Client:
//
//	client := grok.New()
//	comp, err := client.Chat.Create(ctx, &chat.CreateRequest{...})
//
// Key functions:
//   - [Client.Create] — single-turn completion
//   - [Client.Stream] — streaming completion over SSE; iterate with [Stream.Next]
//   - [Client.RunAgent] — automatic tool-call loop until the model stops requesting tools
//   - [NewConversation] — stateful multi-turn helper that carries message history
//   - [Decode] — unmarshal a JSON completion into a typed value T
//
// For observable multi-agent runs with handoffs and guardrails, use the
// agents package instead of [Client.RunAgent].
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/buckedunicorn/grok/internal/idvalidate"
	"github.com/buckedunicorn/grok/internal/transport"
)

// Client wraps the chat completions endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates a chat Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// --- Request types ---

// CreateRequest is the body of POST /v1/chat/completions.
//
// Pointer fields are optional and elided when nil so the wire format
// distinguishes "default" from "explicitly zero". Unknown fields on the
// wire are ignored; missing pointer fields take the API's documented
// default.
type CreateRequest struct {
	Model               string          `json:"model"`
	Messages            []Message       `json:"messages"`
	Stream              bool            `json:"stream,omitempty"`
	Tools               []Tool          `json:"tools,omitempty"`
	ToolChoice          any             `json:"tool_choice,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	MaxCompletionTokens *int            `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string          `json:"reasoning_effort,omitempty"`
	SearchParameters    *SearchParams   `json:"search_parameters,omitempty"`
	ResponseFormat      *ResponseFormat `json:"response_format,omitempty"`
	Seed                *int            `json:"seed,omitempty"`
	N                   *int            `json:"n,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	Stop                []string        `json:"stop,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	FrequencyPenalty    *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64        `json:"presence_penalty,omitempty"`
	LogProbs            *bool           `json:"logprobs,omitempty"`
	TopLogProbs         *int            `json:"top_logprobs,omitempty"`
	User                string          `json:"user,omitempty"`
	Deferred            bool            `json:"deferred,omitempty"`
}

// Message is one entry in the conversation transcript.
//
// Content is either a plain string (the common case) or a []ContentPart
// for multi-modal inputs (text + images). The type assertion happens at
// JSON-encode time, so callers can mix shapes across messages.
type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"` // string or []ContentPart
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	// ReasoningContent is the model's chain-of-thought, returned separately
	// from Content by xAI reasoning models (and OpenAI o1-style providers).
	// Older "<think>...</think>" inline-tag pathways still work via the
	// chat.ThinkingContent helper, which checks this field first and falls
	// back to scanning Content.
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// ContentPart is one element of a multi-modal Message.Content slice.
type ContentPart struct {
	Type     string    `json:"type"` // "text" | "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL references an image to include in a multi-modal message.
// URL is either an https:// link or a data: URI. Detail controls how
// many tokens the model spends interpreting the image.
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "low" | "high" | "auto"
}

// Tool advertises one function the model may call.
type Tool struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef describes a single tool's name, purpose, and JSON Schema.
// Parameters is rendered to JSON as-is, so any value that marshals to a
// valid JSON Schema object is acceptable (a map[string]any literal, a
// hand-built struct, or a *json.RawMessage).
type FunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"` // JSON Schema object
}

// ToolCall is one tool invocation the model emitted on a turn.
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"` // "function"
	Function FunctionCallData `json:"function"`
}

// FunctionCallData carries the function name and the model's JSON-encoded
// argument string. Arguments is a string (not parsed JSON) so callers can
// surface a streamed-but-incomplete fragment without rejecting it.
type FunctionCallData struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded
}

// SearchParams configures live web/X search injection on a chat request.
type SearchParams struct {
	Mode             string   `json:"mode,omitempty"` // "off" | "on" | "auto"
	MaxSearchResults *int     `json:"max_search_results,omitempty"`
	ReturnCitations  *bool    `json:"return_citations,omitempty"`
	Sources          []string `json:"sources,omitempty"`
	FromDate         string   `json:"from_date,omitempty"`
	ToDate           string   `json:"to_date,omitempty"`
}

// ResponseFormat constrains the output shape. Use Type "json_object" for
// any-valid-JSON responses or "json_schema" with a schema in JSONSchema to
// require a specific structure.
type ResponseFormat struct {
	Type       string           `json:"type"` // "text" | "json_object" | "json_schema"
	JSONSchema *json.RawMessage `json:"json_schema,omitempty"`
}

// --- Response types ---

// Completion is the body of a successful POST /v1/chat/completions.
//
// Choices is always non-empty on a 2xx response; the common case is N=1
// and Choices[0].Message holds the assistant reply. Citations is populated
// when SearchParameters.ReturnCitations was true on the request.
type Completion struct {
	ID                string     `json:"id"`
	Object            string     `json:"object"`
	Created           int64      `json:"created"`
	Model             string     `json:"model"`
	Choices           []Choice   `json:"choices"`
	Usage             Usage      `json:"usage"`
	Citations         []Citation `json:"citations"`
	SystemFingerprint string     `json:"system_fingerprint"`
}

// Choice is one alternative completion within a Completion.
//
// FinishReason is one of "stop", "length", "tool_calls",
// "content_filter", or "function_call". Tool-use loops should branch on
// "tool_calls".
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage reports token consumption for the request. CostInUSDTicks is in
// micro-cents (10^-6 USD); divide by 1e8 for dollars.
type Usage struct {
	PromptTokens            int                     `json:"prompt_tokens"`
	CompletionTokens        int                     `json:"completion_tokens"`
	TotalTokens             int                     `json:"total_tokens"`
	PromptTokensDetails     PromptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokensDetails CompletionTokensDetails `json:"completion_tokens_details"`
	NumSourcesUsed          int                     `json:"num_sources_used"`
	CostInUSDTicks          int64                   `json:"cost_in_usd_ticks"`
}

// PromptTokensDetails breaks the prompt's token count down by modality.
// CachedTokens is the slice the API served from its prompt-prefix cache;
// see the cache package for tuning conv-id.
type PromptTokensDetails struct {
	TextTokens   int `json:"text_tokens"`
	AudioTokens  int `json:"audio_tokens"`
	ImageTokens  int `json:"image_tokens"`
	CachedTokens int `json:"cached_tokens"`
}

// CompletionTokensDetails breaks the completion's token count down by
// kind. ReasoningTokens count toward billing but never appear in the
// visible Message.Content; they are the model's hidden chain-of-thought.
type CompletionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AudioTokens              int `json:"audio_tokens"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
}

// Citation is one source the model consulted via live search.
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// Chunk is a single SSE delta from a streaming completion.
type Chunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage"` // only in final chunk when stream_options.include_usage=true
}

// ChunkChoice is a streaming-mode counterpart to Choice. Delta carries
// the incremental piece for this chunk; FinishReason is empty until the
// final chunk for this index.
type ChunkChoice struct {
	Index        int    `json:"index"`
	Delta        Delta  `json:"delta"`
	FinishReason string `json:"finish_reason"`
}

// Delta is the incremental update inside one streaming chunk. The first
// chunk for a choice carries Role; subsequent chunks accumulate Content.
// Tool-call deltas may stream the Arguments field across multiple chunks.
type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// DeferredResponse holds only the request_id returned when deferred=true.
type DeferredResponse struct {
	RequestID string `json:"request_id"`
}

// --- Stream ---

// Stream wraps an SSE connection for a streaming chat completion.
type Stream struct{ s *transport.SSEStream }

// Next returns the next chunk. Returns io.EOF when the stream is complete.
func (s *Stream) Next() (*Chunk, error) {
	data, err := s.s.Next()
	if err != nil {
		return nil, err
	}
	var chunk Chunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, err
	}
	return &chunk, nil
}

// Close releases the underlying HTTP response body.
func (s *Stream) Close() error { return s.s.Close() }

// Collect reads all chunks and accumulates the full content string.
//
// Only the first choice's content is collected; if the request was made
// with N > 1, the additional choices are silently discarded. To consume
// every choice or to read the final-chunk Usage, call Stream.Next
// directly.
func (s *Stream) Collect() (string, error) {
	defer s.Close()
	var sb strings.Builder
	for {
		chunk, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if len(chunk.Choices) > 0 {
			sb.WriteString(chunk.Choices[0].Delta.Content)
		}
	}
	return sb.String(), nil
}

// --- Methods ---

// Create sends a chat completion request and returns the full response.
// The caller's *CreateRequest is not mutated; the
// Stream flag is overridden on a shallow copy.
func (c *Client) Create(ctx context.Context, req *CreateRequest) (*Completion, error) {
	r := *req
	r.Stream = false
	var out Completion
	if err := c.t.Do(ctx, "POST", "/v1/chat/completions", &r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Stream sends a streaming chat completion request.
// Use the returned Stream.Next() to read chunks. Call Stream.Close() when done.
// The caller's *CreateRequest is not mutated.
func (c *Client) Stream(ctx context.Context, req *CreateRequest) (*Stream, error) {
	r := *req
	r.Stream = true
	s, err := c.t.Stream(ctx, "POST", "/v1/chat/completions", &r)
	if err != nil {
		return nil, err
	}
	return &Stream{s: s}, nil
}

// CreateDeferred submits a deferred request and returns the request_id for polling.
// The caller's *CreateRequest is not mutated.
func (c *Client) CreateDeferred(ctx context.Context, req *CreateRequest) (string, error) {
	r := *req
	r.Deferred = true
	r.Stream = false
	var out DeferredResponse
	if err := c.t.Do(ctx, "POST", "/v1/chat/completions", &r, &out); err != nil {
		return "", err
	}
	return out.RequestID, nil
}

// GetDeferred polls GET /v1/chat/deferred-completion/{id} for a deferred result.
// Returns the completion when ready. A 202 response means still pending, retry.
func (c *Client) GetDeferred(ctx context.Context, requestID string) (*Completion, error) {
	if err := idvalidate.OpaqueID("grok/chat", "requestID", requestID); err != nil {
		return nil, err
	}
	var out Completion
	if err := c.t.Do(ctx, "GET", "/v1/chat/deferred-completion/"+url.PathEscape(requestID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
