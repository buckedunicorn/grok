// Package ttsstream provides a WebSocket client for streaming text-to-speech.
//
// The streaming TTS endpoint (wss://api.x.ai/v1/tts) accepts text incrementally
// and returns audio chunks in real time. A single connection can synthesize
// multiple utterances sequentially.
//
// Usage:
//
//	s, err := ttsstream.Dial(ctx, apiKey, ttsstream.WithVoice("eve"), ttsstream.WithLanguage("en"))
//	audio := s.Audio() // receive audio chunks
//	s.SendText(ctx, "Hello, world!")
//	s.Done(ctx)
//	for chunk := range audio { /* play chunk */ }
//	s.Close()
package ttsstream

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const ttsURL = "wss://api.x.ai/v1/tts"

// Stream is an open streaming TTS WebSocket connection.
type Stream struct {
	conn    *websocket.Conn
	audioCh chan []byte
	errCh   chan error
}

// StreamOption configures a Dial call.
type StreamOption func(*streamConfig)

type streamConfig struct {
	voice             string
	language          string
	codec             string
	sampleRate        int
	bitRate           int
	optimizeLatency   int
	textNormalization bool
}

// WithVoice sets the voice (default: eve).
func WithVoice(v string) StreamOption { return func(c *streamConfig) { c.voice = v } }

// WithLanguage sets the BCP-47 language code (required, e.g. "en").
func WithLanguage(l string) StreamOption { return func(c *streamConfig) { c.language = l } }

// WithCodec sets the audio codec (default: mp3). Options: mp3, wav, pcm, mulaw, alaw.
func WithCodec(codec string) StreamOption { return func(c *streamConfig) { c.codec = codec } }

// WithSampleRate sets the sample rate in Hz (default: 24000).
func WithSampleRate(r int) StreamOption { return func(c *streamConfig) { c.sampleRate = r } }

// WithOptimizeLatency enables reduced-latency mode (1) at slight quality cost.
func WithOptimizeLatency(level int) StreamOption {
	return func(c *streamConfig) { c.optimizeLatency = level }
}

// WithTextNormalization enables spoken-form normalization of numbers and abbreviations.
func WithTextNormalization(v bool) StreamOption {
	return func(c *streamConfig) { c.textNormalization = v }
}

// Dial connects to the streaming TTS WebSocket and starts the receive loop.
func Dial(ctx context.Context, apiKey string, opts ...StreamOption) (*Stream, error) {
	cfg := &streamConfig{
		voice:    "eve",
		language: "en",
		codec:    "mp3",
	}
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
		return nil, fmt.Errorf("ttsstream.Dial: %w", err)
	}
	s := &Stream{
		conn:    conn,
		audioCh: make(chan []byte, 64),
		errCh:   make(chan error, 1),
	}
	go s.readLoop(ctx)
	return s, nil
}

// SendText sends a chunk of text to be synthesized.
// Audio generation starts as soon as enough text is buffered.
func (s *Stream) SendText(ctx context.Context, text string) error {
	return wsjson.Write(ctx, s.conn, map[string]any{
		"type":  "text.delta",
		"delta": text,
	})
}

// Done signals that all text for this utterance has been sent.
// After the audio.done event, you can call SendText again for the next utterance.
func (s *Stream) Done(ctx context.Context) error {
	return wsjson.Write(ctx, s.conn, map[string]string{"type": "text.done"})
}

// Audio returns the channel on which decoded audio chunks are delivered.
// The channel is closed when audio generation is complete or the connection closes.
func (s *Stream) Audio() <-chan []byte { return s.audioCh }

// Err returns the first error encountered by the receive loop (nil if clean close).
func (s *Stream) Err() error {
	select {
	case err := <-s.errCh:
		return err
	default:
		return nil
	}
}

// Close closes the WebSocket connection.
func (s *Stream) Close() error {
	return s.conn.Close(websocket.StatusNormalClosure, "")
}

func (s *Stream) readLoop(ctx context.Context) {
	defer close(s.audioCh)
	for {
		_, msg, err := s.conn.Read(ctx)
		if err != nil {
			s.errCh <- err
			return
		}
		var ev struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(msg, &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "audio.delta":
			b, err := base64.StdEncoding.DecodeString(ev.Delta)
			if err != nil {
				continue
			}
			select {
			case s.audioCh <- b:
			case <-ctx.Done():
				return
			}
		case "audio.done":
			// utterance complete; channel stays open for multi-utterance
		case "error":
			s.errCh <- fmt.Errorf("ttsstream: server error: %s", msg)
			return
		}
	}
}

func buildURL(cfg *streamConfig) string {
	u := ttsURL + "?language=" + cfg.language
	if cfg.voice != "" {
		u += "&voice=" + cfg.voice
	}
	if cfg.codec != "" {
		u += "&codec=" + cfg.codec
	}
	if cfg.sampleRate > 0 {
		u += fmt.Sprintf("&sample_rate=%d", cfg.sampleRate)
	}
	if cfg.bitRate > 0 {
		u += fmt.Sprintf("&bit_rate=%d", cfg.bitRate)
	}
	if cfg.optimizeLatency != 0 {
		u += fmt.Sprintf("&optimize_streaming_latency=%d", cfg.optimizeLatency)
	}
	if cfg.textNormalization {
		u += "&text_normalization=true"
	}
	return u
}
