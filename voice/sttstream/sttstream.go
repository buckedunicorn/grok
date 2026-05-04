// Package sttstream provides a WebSocket client for streaming speech-to-text.
//
// The streaming STT endpoint (wss://api.x.ai/v1/stt) accepts raw audio binary
// frames and returns transcript events in real time. Each connection handles
// a single utterance; reconnect for the next one.
//
// Usage:
//
//	s, err := sttstream.Dial(ctx, apiKey, sttstream.WithLanguage("en"), sttstream.WithInterimResults(true))
//	<-s.Ready() // wait for transcript.created before sending audio
//	events := s.Listen(ctx)
//	s.SendAudio(ctx, pcmChunk)
//	s.Done(ctx)
//	for ev := range events { fmt.Println(ev.Text) }
package sttstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
)

const sttURL = "wss://api.x.ai/v1/stt"

// Stream is an open streaming STT WebSocket connection.
type Stream struct {
	conn    *websocket.Conn
	readyCh chan struct{}
}

// StreamOption configures a Dial call.
type StreamOption func(*streamConfig)

type streamConfig struct {
	sampleRate     int
	encoding       string
	interimResults bool
	endpointing    int
	language       string
	multichannel   bool
	channels       int
	diarize        bool
}

// WithSampleRate sets the audio sample rate in Hz (default: 16000).
func WithSampleRate(r int) StreamOption { return func(c *streamConfig) { c.sampleRate = r } }

// WithEncoding sets the audio encoding (default: pcm). Options: pcm, mulaw, alaw.
func WithEncoding(enc string) StreamOption { return func(c *streamConfig) { c.encoding = enc } }

// WithInterimResults enables partial transcript events (~500ms intervals).
func WithInterimResults(v bool) StreamOption { return func(c *streamConfig) { c.interimResults = v } }

// WithEndpointing sets silence duration (ms) before a speech_final event (default: 10).
func WithEndpointing(ms int) StreamOption { return func(c *streamConfig) { c.endpointing = ms } }

// WithLanguage sets the BCP-47 language code for transcription and normalization.
func WithLanguage(l string) StreamOption { return func(c *streamConfig) { c.language = l } }

// WithMultichannel enables per-channel transcription (requires Channels ≥ 2).
func WithMultichannel(v bool) StreamOption { return func(c *streamConfig) { c.multichannel = v } }

// WithChannels sets the number of interleaved audio channels for multichannel mode.
func WithChannels(n int) StreamOption { return func(c *streamConfig) { c.channels = n } }

// WithDiarize enables speaker diarization.
func WithDiarize(v bool) StreamOption { return func(c *streamConfig) { c.diarize = v } }

// TranscriptEvent is a server message from the streaming STT endpoint.
type TranscriptEvent struct {
	// Type is one of: "transcript.created", "transcript.partial", "transcript.done", "error".
	Type        string           `json:"type"`
	Text        string           `json:"text"`
	IsFinal     bool             `json:"is_final"`
	SpeechFinal bool             `json:"speech_final"`
	Duration    float64          `json:"duration"`
	Words       []TranscriptWord `json:"words"`
	Channels    []ChannelResult  `json:"channels"`
	Error       string           `json:"error,omitempty"`
}

type TranscriptWord struct {
	Text       string  `json:"text"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Confidence float64 `json:"confidence"`
	Speaker    *int    `json:"speaker"`
}

type ChannelResult struct {
	Index    int              `json:"index"`
	Language string           `json:"language"`
	Text     string           `json:"text"`
	Words    []TranscriptWord `json:"words"`
}

// Dial connects to the streaming STT WebSocket.
func Dial(ctx context.Context, apiKey string, opts ...StreamOption) (*Stream, error) {
	cfg := &streamConfig{encoding: "pcm"}
	for _, o := range opts {
		o(cfg)
	}
	u := buildURL(cfg)
	conn, _, err := websocket.Dial(ctx, u, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + apiKey},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("sttstream.Dial: %w", err)
	}
	s := &Stream{conn: conn, readyCh: make(chan struct{})}
	return s, nil
}

// Ready returns a channel that is closed once the server sends "transcript.created".
// Send audio only after this fires.
func (s *Stream) Ready() <-chan struct{} { return s.readyCh }

// SendAudio sends a raw audio binary frame (no base64; direct bytes).
func (s *Stream) SendAudio(ctx context.Context, audio []byte) error {
	return s.conn.Write(ctx, websocket.MessageBinary, audio)
}

// Done signals that all audio has been sent. The server flushes and closes after transcript.done.
func (s *Stream) Done(ctx context.Context) error {
	msg, _ := json.Marshal(map[string]string{"type": "audio.done"})
	return s.conn.Write(ctx, websocket.MessageText, msg)
}

// Listen reads server events in a background goroutine and returns a channel.
// The channel is closed after "transcript.done" or on connection close.
// This also handles signalling Ready() on "transcript.created".
func (s *Stream) Listen(ctx context.Context) <-chan TranscriptEvent {
	ch := make(chan TranscriptEvent, 32)
	readyOnce := make(chan struct{}, 1)
	go func() {
		defer close(ch)
		for {
			_, msg, err := s.conn.Read(ctx)
			if err != nil {
				return
			}
			var ev TranscriptEvent
			if err := json.Unmarshal(msg, &ev); err != nil {
				continue
			}
			if ev.Type == "transcript.created" {
				select {
				case readyOnce <- struct{}{}:
					close(s.readyCh)
				default:
				}
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
			if ev.Type == "transcript.done" {
				return
			}
		}
	}()
	return ch
}

// Close closes the WebSocket connection.
func (s *Stream) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

func buildURL(cfg *streamConfig) string {
	u := sttURL
	sep := "?"
	add := func(k, v string) {
		if v == "" {
			return
		}
		u += sep + k + "=" + v
		sep = "&"
	}
	if cfg.sampleRate > 0 {
		add("sample_rate", fmt.Sprintf("%d", cfg.sampleRate))
	}
	add("encoding", cfg.encoding)
	if cfg.interimResults {
		add("interim_results", "true")
	}
	if cfg.endpointing > 0 {
		add("endpointing", fmt.Sprintf("%d", cfg.endpointing))
	}
	add("language", cfg.language)
	if cfg.multichannel {
		add("multichannel", "true")
	}
	if cfg.channels > 0 {
		add("channels", fmt.Sprintf("%d", cfg.channels))
	}
	if cfg.diarize {
		add("diarize", "true")
	}
	return u
}
