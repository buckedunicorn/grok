// Package voice provides TTS, STT, and realtime voice endpoints.
// For WebSocket-based streaming, use the sub-packages:
//   - voice/realtime, bidirectional voice conversation
//   - voice/ttsstream, streaming text-to-speech
//   - voice/sttstream, streaming speech-to-text
package voice

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/buckedunicorn/grok/internal/idvalidate"
	"github.com/buckedunicorn/grok/internal/transport"
)

// Client wraps the voice endpoints.
type Client struct{ t *transport.Transport }

// NewClient creates a voice Client.
func NewClient(t *transport.Transport) *Client { return &Client{t: t} }

// --- TTS ---

// TTSRequest is the body for POST /v1/tts.
type TTSRequest struct {
	Text                     string       `json:"text"`
	VoiceID                  string       `json:"voice_id,omitempty"` // default: "eve"
	Language                 string       `json:"language"`
	OutputFormat             *AudioFormat `json:"output_format,omitempty"`
	OptimizeStreamingLatency string       `json:"optimize_streaming_latency,omitempty"` // "0" | "1"
	TextNormalization        *bool        `json:"text_normalization,omitempty"`
}

// AudioFormat selects the encoding of generated TTS audio. Codec picks
// the container; SampleRate and BitRate are codec-specific (BitRate
// applies only to MP3).
type AudioFormat struct {
	Codec      string `json:"codec"` // "mp3" | "wav" | "pcm" | "mulaw" | "alaw"
	SampleRate *int   `json:"sample_rate,omitempty"`
	BitRate    *int   `json:"bit_rate,omitempty"` // MP3 only
}

// Voice is one available TTS voice. Pass VoiceID to TTSRequest.VoiceID
// to render speech in that voice.
type Voice struct {
	VoiceID  string  `json:"voice_id"`
	Name     string  `json:"name"`
	Language *string `json:"language"`
}

// TextToSpeech calls POST /v1/tts and returns the raw audio bytes.
func (c *Client) TextToSpeech(ctx context.Context, req *TTSRequest) ([]byte, error) {
	return c.t.DoRaw(ctx, "POST", "/v1/tts", req)
}

// ListVoices calls GET /v1/tts/voices and returns all available voices.
func (c *Client) ListVoices(ctx context.Context) ([]Voice, error) {
	var out struct {
		Voices []Voice `json:"voices"`
	}
	if err := c.t.Do(ctx, "GET", "/v1/tts/voices", nil, &out); err != nil {
		return nil, err
	}
	return out.Voices, nil
}

// GetVoice returns details for a specific voice by ID.
func (c *Client) GetVoice(ctx context.Context, voiceID string) (*Voice, error) {
	if err := idvalidate.OpaqueID("grok/voice", "voiceID", voiceID); err != nil {
		return nil, err
	}
	var out Voice
	if err := c.t.Do(ctx, "GET", "/v1/tts/voices/"+url.PathEscape(voiceID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- STT ---

// STTRequest configures a speech-to-text transcription.
type STTRequest struct {
	// Provide either File+Filename or URL.
	File         io.Reader
	Filename     string
	URL          string
	Language     string
	Format       bool // enable text formatting (requires Language)
	Diarize      bool // speaker diarization
	Multichannel bool
	Channels     int    // required for multichannel raw audio
	AudioFormat  string // only for raw formats: "pcm" | "mulaw" | "alaw"
	SampleRate   string // only for raw formats: "8000" | "16000" | etc.
}

// Transcript is the response from POST /v1/stt.
type Transcript struct {
	Text     string              `json:"text"`
	Language string              `json:"language"`
	Duration float64             `json:"duration"`
	Words    []TranscriptWord    `json:"words"`
	Channels []ChannelTranscript `json:"channels"`
}

// TranscriptWord is one word in a transcript with its time bounds.
// Speaker is non-nil when STTRequest.Diarize was true.
type TranscriptWord struct {
	Text       string  `json:"text"`
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Confidence float64 `json:"confidence"`
	Speaker    *int    `json:"speaker"`
}

// ChannelTranscript is one channel's transcript when STTRequest.Multichannel
// was true. The top-level Transcript.Text concatenates across channels;
// per-channel text and words live here.
type ChannelTranscript struct {
	Index    int              `json:"index"`
	Language string           `json:"language"`
	Text     string           `json:"text"`
	Words    []TranscriptWord `json:"words"`
}

// Transcribe calls POST /v1/stt using multipart/form-data.
func (c *Client) Transcribe(ctx context.Context, req *STTRequest) (*Transcript, error) {
	fields := buildSTTFields(req)
	var out Transcript
	if err := c.t.DoMultipart(ctx, "/v1/stt", fields, "file", req.Filename, req.File, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func buildSTTFields(req *STTRequest) map[string]string {
	f := map[string]string{}
	if req.URL != "" {
		f["url"] = req.URL
	}
	if req.Language != "" {
		f["language"] = req.Language
	}
	if req.Format {
		f["format"] = "true"
	}
	if req.Diarize {
		f["diarize"] = "true"
	}
	if req.Multichannel {
		f["multichannel"] = "true"
	}
	if req.Channels > 0 {
		f["channels"] = fmt.Sprintf("%d", req.Channels)
	}
	if req.AudioFormat != "" {
		f["audio_format"] = req.AudioFormat
	}
	if req.SampleRate != "" {
		f["sample_rate"] = req.SampleRate
	}
	return f
}

// --- Realtime client secret ---

// EphemeralTokenRequest is the body for POST /v1/realtime/client_secrets.
type EphemeralTokenRequest struct {
	ExpiresAfter *ExpiresAfter    `json:"expires_after,omitempty"`
	Session      *RealtimeSession `json:"session,omitempty"`
}

// ExpiresAfter sets the lifetime of an ephemeral realtime token in
// seconds. Defaults to 60 when omitted; max 600.
type ExpiresAfter struct {
	Seconds int `json:"seconds"`
}

// RealtimeSession pre-configures the realtime session the ephemeral
// token unlocks. Model selects which voice model the eventual WebSocket
// connection will use.
type RealtimeSession struct {
	Model string `json:"model,omitempty"` // "grok-voice-fast-1.0" | "grok-voice-think-fast-1.0"
}

// EphemeralToken is a short-lived bearer credential for the realtime
// WebSocket. Treat it like a password; it cannot be revoked early.
// ExpiresAt is a Unix epoch second.
type EphemeralToken struct {
	Value     string `json:"value"`
	ExpiresAt int64  `json:"expires_at"`
}

// CreateEphemeralToken creates a short-lived client secret for WebSocket realtime connections.
func (c *Client) CreateEphemeralToken(ctx context.Context, req *EphemeralTokenRequest) (*EphemeralToken, error) {
	var out EphemeralToken
	if err := c.t.Do(ctx, "POST", "/v1/realtime/client_secrets", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
