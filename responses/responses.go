// Package responses provides the /v1/responses endpoint (xAI Responses API).
// This is the newer, stateful API that chains turns via previous_response_id.
package responses

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

// Client wraps the Responses API endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates a responses Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// --- Request types ---

// CreateRequest is the body of POST /v1/responses.
//
// Input is either a plain string (the common case) or a []InputItem for
// multi-turn or multi-modal inputs. Set PreviousResponseID to chain a
// follow-up turn against an earlier server-side state instead of resending
// history. Reasoning, MaxTurns, and the server-side tools (web_search,
// x_search, code_interpreter advertised via Tools) are only available on
// this surface, not on chat.CreateRequest.
type CreateRequest struct {
	Model              string        `json:"model"`
	Input              any           `json:"input"` // string or []InputItem
	Instructions       string        `json:"instructions,omitempty"`
	PreviousResponseID string        `json:"previous_response_id,omitempty"`
	Stream             bool          `json:"stream,omitempty"`
	Tools              []Tool        `json:"tools,omitempty"`
	ToolChoice         any           `json:"tool_choice,omitempty"`
	Reasoning          *Reasoning    `json:"reasoning,omitempty"`
	Temperature        *float64      `json:"temperature,omitempty"`
	MaxOutputTokens    *int          `json:"max_output_tokens,omitempty"`
	MaxTurns           *int          `json:"max_turns,omitempty"`
	TopP               *float64      `json:"top_p,omitempty"`
	Store              *bool         `json:"store,omitempty"`
	User               string        `json:"user,omitempty"`
	ParallelToolCalls  *bool         `json:"parallel_tool_calls,omitempty"`
	SearchParameters   *SearchParams `json:"search_parameters,omitempty"`
	Text               *TextConfig   `json:"text,omitempty"`
}

// InputItem is one entry in a Responses-API input list. Role+Content
// describe a user/assistant message; Type+CallID+Output describe a
// function-call result threaded back into the conversation.
type InputItem struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []ContentPart
	Type    string `json:"type,omitempty"`
	CallID  string `json:"call_id,omitempty"`
	Output  string `json:"output,omitempty"`
}

// Tool advertises one tool the model may call. Type "function" is a
// caller-supplied function; the other types ("web_search", "x_search",
// "code_interpreter") select server-side tools that xAI runs for you.
type Tool struct {
	Type        string `json:"type"` // "function" | "web_search" | "x_search" | etc.
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// Reasoning controls how much hidden chain-of-thought the model produces.
// Higher effort spends more reasoning tokens (which still bill) for
// better answers on hard problems.
type Reasoning struct {
	Effort string `json:"effort,omitempty"` // "low" | "medium" | "high"
}

// SearchParams configures live web/X search injection on a responses
// request, mirroring chat.SearchParams.
type SearchParams struct {
	Mode             string   `json:"mode,omitempty"`
	MaxSearchResults *int     `json:"max_search_results,omitempty"`
	ReturnCitations  *bool    `json:"return_citations,omitempty"`
	Sources          []string `json:"sources,omitempty"`
	FromDate         string   `json:"from_date,omitempty"`
	ToDate           string   `json:"to_date,omitempty"`
}

// TextConfig groups text-output settings. Format constrains the output
// shape, mirroring chat.ResponseFormat.
type TextConfig struct {
	Format *FormatConfig `json:"format,omitempty"`
}

// FormatConfig constrains the output to plain text, any-valid-JSON, or
// a specific JSON Schema.
type FormatConfig struct {
	Type       string           `json:"type"` // "text" | "json_object" | "json_schema"
	JSONSchema *json.RawMessage `json:"json_schema,omitempty"`
}

// --- Response types ---

// Response is the body of a successful POST /v1/responses or GET
// /v1/responses/{id}. Status is "completed" for terminal responses;
// in-progress and incomplete responses appear when a run was streamed
// or stopped early.
type Response struct {
	ID                 string       `json:"id"`
	Object             string       `json:"object"`
	Model              string       `json:"model"`
	CreatedAt          int64        `json:"created_at"`
	CompletedAt        *int64       `json:"completed_at"`
	Status             string       `json:"status"` // "completed" | "in_progress" | "incomplete"
	Output             []OutputItem `json:"output"`
	PreviousResponseID *string      `json:"previous_response_id"`
	Reasoning          *Reasoning   `json:"reasoning"`
	Store              bool         `json:"store"`
	Temperature        *float64     `json:"temperature"`
	Usage              *Usage       `json:"usage"`
	User               *string      `json:"user"`
	Error              *ErrorDetail `json:"error"`
}

// OutputItem is one element in Response.Output. Type discriminates the
// shape: "message" carries assistant text in Content, "reasoning" carries
// hidden chain-of-thought in Summary, "function_call" carries the tool
// invocation in Name / CallID / Arguments.
type OutputItem struct {
	Type    string          `json:"type"` // "message" | "reasoning" | "function_call" | etc.
	ID      string          `json:"id"`
	Role    string          `json:"role,omitempty"`
	Status  string          `json:"status,omitempty"`
	Content []OutputContent `json:"content,omitempty"`
	Summary []SummaryItem   `json:"summary,omitempty"`
	// Function call fields
	Name      string `json:"name,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// OutputText returns the concatenated text content from a message output item.
func (o *OutputItem) OutputText() string {
	var sb strings.Builder
	for _, c := range o.Content {
		if c.Type == "output_text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

// OutputContent is one piece of a message OutputItem's content. Type
// distinguishes plain "output_text" from typed annotation envelopes.
type OutputContent struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Annotations []any  `json:"annotations,omitempty"`
}

// SummaryItem is one bullet inside a "reasoning" OutputItem.
type SummaryItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage reports token consumption for a Responses request. CostInUSDTicks
// is in micro-cents (10^-6 USD); divide by 1e8 for dollars.
type Usage struct {
	InputTokens            int                 `json:"input_tokens"`
	OutputTokens           int                 `json:"output_tokens"`
	TotalTokens            int                 `json:"total_tokens"`
	InputTokensDetails     InputTokensDetails  `json:"input_tokens_details"`
	OutputTokensDetails    OutputTokensDetails `json:"output_tokens_details"`
	NumSourcesUsed         int                 `json:"num_sources_used"`
	NumServerSideToolsUsed int                 `json:"num_server_side_tools_used"`
	CostInUSDTicks         *int64              `json:"cost_in_usd_ticks"`
}

// InputTokensDetails breaks the input token count down. CachedTokens is
// the slice the API served from its prompt-prefix cache.
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// OutputTokensDetails breaks the output token count down. ReasoningTokens
// counts hidden chain-of-thought that bills but never appears in
// OutputItem.Content.
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ErrorDetail is the per-response failure when Status is not "completed".
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OutputText returns the text from the first message output item in a response.
func (r *Response) OutputText() string {
	for _, item := range r.Output {
		if item.Type == "message" {
			return item.OutputText()
		}
	}
	return ""
}

// --- Stream ---

// StreamEvent is one SSE event from a streaming Responses run. Type
// discriminates the event shape; Delta carries an incremental payload
// (raw JSON) and Response is set on the terminal response.completed
// event.
type StreamEvent struct {
	Type  string          `json:"type"`
	Delta json.RawMessage `json:"delta,omitempty"`
	// Full response on response.completed
	Response *Response `json:"response,omitempty"`
}

// Stream wraps an SSE connection for a streaming Responses API call.
type Stream struct{ s *transport.SSEStream }

// Next returns the next event. Returns io.EOF when done.
func (s *Stream) Next() (*StreamEvent, error) {
	data, err := s.s.Next()
	if err != nil {
		return nil, err
	}
	var event StreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

// Close releases the underlying HTTP response body.
func (s *Stream) Close() error { return s.s.Close() }

// Collect reads all events and returns the accumulated output text.
func (s *Stream) Collect() (string, error) {
	defer s.Close()
	var text string
	for {
		ev, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if ev.Type == "response.completed" && ev.Response != nil {
			return ev.Response.OutputText(), nil
		}
	}
	return text, nil
}

// --- Methods ---

// Create sends a Responses API request and returns the full response.
// The caller's *CreateRequest is not mutated.
func (c *Client) Create(ctx context.Context, req *CreateRequest) (*Response, error) {
	r := *req
	r.Stream = false
	var out Response
	if err := c.t.Do(ctx, "POST", "/v1/responses", &r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Stream sends a streaming Responses API request.
// The caller's *CreateRequest is not mutated.
func (c *Client) Stream(ctx context.Context, req *CreateRequest) (*Stream, error) {
	r := *req
	r.Stream = true
	s, err := c.t.Stream(ctx, "POST", "/v1/responses", &r)
	if err != nil {
		return nil, err
	}
	return &Stream{s: s}, nil
}

// Get retrieves a previously generated response by ID.
func (c *Client) Get(ctx context.Context, responseID string) (*Response, error) {
	if err := validateOpaqueID("responseID", responseID); err != nil {
		return nil, err
	}
	var out Response
	if err := c.t.Do(ctx, "GET", "/v1/responses/"+url.PathEscape(responseID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete removes a previously generated response.
func (c *Client) Delete(ctx context.Context, responseID string) error {
	if err := validateOpaqueID("responseID", responseID); err != nil {
		return err
	}
	return c.t.Do(ctx, "DELETE", "/v1/responses/"+url.PathEscape(responseID), nil, nil)
}

// validateOpaqueID delegates to the shared idvalidate helper.
func validateOpaqueID(field, id string) error {
	return idvalidate.OpaqueID("grok/responses", field, id)
}
