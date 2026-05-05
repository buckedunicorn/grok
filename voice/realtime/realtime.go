// Package realtime provides a WebSocket client for the xAI Realtime voice API.
//
// The realtime endpoint (wss://api.x.ai/v1/realtime) enables low-latency
// bidirectional voice conversations with Grok models.
//
// Usage:
//
//	sess, err := realtime.Dial(ctx, apiKey, realtime.WithModel("grok-voice-think-fast-1.0"))
//	events, err := sess.Listen(ctx)
//	// send audio...
//	sess.AppendAudio(ctx, audioChunk)
//	sess.CommitAudio(ctx)
//	// read events from channel
//	for ev := range events { ... }
package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const realtimeURL = "wss://api.x.ai/v1/realtime"

// Session is an open Realtime API WebSocket connection.
type Session struct {
	conn   *websocket.Conn
	apiKey string
}

// SessionOption configures a Dial call.
type SessionOption func(*sessionConfig)

type sessionConfig struct {
	model string
}

// WithModel sets the model for the session (default: grok-voice-fast-1.0).
// Use grok-voice-think-fast-1.0 for better quality.
func WithModel(model string) SessionOption {
	return func(c *sessionConfig) { c.model = model }
}

// Dial connects to the Realtime API WebSocket endpoint and returns an open Session.
func Dial(ctx context.Context, apiKey string, opts ...SessionOption) (*Session, error) {
	cfg := &sessionConfig{model: "grok-voice-fast-1.0"}
	for _, o := range opts {
		o(cfg)
	}
	url := realtimeURL + "?model=" + cfg.model
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + apiKey},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("realtime.Dial: %w", err)
	}
	return &Session{conn: conn, apiKey: apiKey}, nil
}

// DialWithEphemeralToken connects using an ephemeral client secret in the
// sec-websocket-protocol header (for browser-side use).
func DialWithEphemeralToken(ctx context.Context, token string, opts ...SessionOption) (*Session, error) {
	cfg := &sessionConfig{model: "grok-voice-fast-1.0"}
	for _, o := range opts {
		o(cfg)
	}
	url := realtimeURL + "?model=" + cfg.model
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		Subprotocols: []string{"xai-client-secret." + token},
	})
	if err != nil {
		return nil, fmt.Errorf("realtime.DialWithEphemeralToken: %w", err)
	}
	return &Session{conn: conn}, nil
}

// --- Client messages ---

// Update sends a session.update message to configure the session.
func (s *Session) Update(ctx context.Context, config SessionConfig) error {
	return s.send(ctx, map[string]any{
		"type":    "session.update",
		"session": config,
	})
}

// SessionConfig holds parameters for a session.update message.
type SessionConfig struct {
	Model             string `json:"model,omitempty"`
	Voice             string `json:"voice,omitempty"`
	Instructions      string `json:"instructions,omitempty"`
	TurnDetection     any    `json:"turn_detection,omitempty"` // nil to disable VAD
	Tools             []Tool `json:"tools,omitempty"`
	InputAudioFormat  string `json:"input_audio_format,omitempty"`
	OutputAudioFormat string `json:"output_audio_format,omitempty"`
}

// Tool advertises one function the realtime model may call. Mirrors
// chat.Tool with a flatter shape; the realtime API does not nest the
// function definition the way chat completions do.
type Tool struct {
	Type        string `json:"type"` // "function"
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// AppendAudio sends a chunk of raw audio bytes to the input buffer (base64-encoded internally).
func (s *Session) AppendAudio(ctx context.Context, audio []byte) error {
	return s.send(ctx, map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64.StdEncoding.EncodeToString(audio),
	})
}

// CommitAudio commits the audio buffer as a user message.
// Only call this when turn_detection is disabled.
func (s *Session) CommitAudio(ctx context.Context) error {
	return s.send(ctx, map[string]string{"type": "input_audio_buffer.commit"})
}

// ClearAudio discards buffered audio without committing it.
func (s *Session) ClearAudio(ctx context.Context) error {
	return s.send(ctx, map[string]string{"type": "input_audio_buffer.clear"})
}

// RequestResponse asks the server to generate an assistant response.
// In VAD mode this is automatic; call manually when turn_detection is nil.
func (s *Session) RequestResponse(ctx context.Context) error {
	return s.send(ctx, map[string]string{"type": "response.create"})
}

// CancelResponse cancels an in-progress response.
func (s *Session) CancelResponse(ctx context.Context) error {
	return s.send(ctx, map[string]string{"type": "response.cancel"})
}

// SendText injects a text message as a conversation item.
func (s *Session) SendText(ctx context.Context, role, text string) error {
	return s.send(ctx, map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": role,
			"content": []map[string]string{
				{"type": "input_text", "text": text},
			},
		},
	})
}

// SendToolResult returns the output of a function call to the model.
func (s *Session) SendToolResult(ctx context.Context, callID, output string) error {
	return s.send(ctx, map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": callID,
			"output":  output,
		},
	})
}

// --- Server events ---

// Event is a server-sent message from the Realtime API.
type Event struct {
	Type string          `json:"type"`
	Raw  json.RawMessage // full JSON for type-specific fields
}

// AudioDelta decodes the base64 audio bytes from a response.output_audio.delta event.
func (e *Event) AudioDelta() ([]byte, error) {
	var v struct {
		Delta string `json:"delta"`
	}
	if err := json.Unmarshal(e.Raw, &v); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(v.Delta)
}

// TranscriptDelta extracts the text from a response.output_audio_transcript.delta event.
func (e *Event) TranscriptDelta() string {
	var v struct {
		Delta string `json:"delta"`
	}
	_ = json.Unmarshal(e.Raw, &v)
	return v.Delta
}

// FunctionCall extracts function name and arguments from a response.function_call_arguments.done event.
func (e *Event) FunctionCall() (name, callID, arguments string) {
	var v struct {
		Name      string `json:"name"`
		CallID    string `json:"call_id"`
		Arguments string `json:"arguments"`
	}
	_ = json.Unmarshal(e.Raw, &v)
	return v.Name, v.CallID, v.Arguments
}

// Listen starts reading server events in a background goroutine and returns a channel.
// The channel is closed when the connection closes or the context is cancelled.
func (s *Session) Listen(ctx context.Context) (<-chan Event, error) {
	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		for {
			_, msg, err := s.conn.Read(ctx)
			if err != nil {
				return
			}
			var ev Event
			if err := json.Unmarshal(msg, &ev); err != nil {
				continue
			}
			ev.Raw = msg
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// Close closes the WebSocket connection with a normal closure.
func (s *Session) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

func (s *Session) send(ctx context.Context, v any) error {
	return wsjson.Write(ctx, s.conn, v)
}
