package voice_test

import (
	"context"
	"os"
	"testing"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/voice"
)

func client(t *testing.T) *grok.Client {
	t.Helper()
	if os.Getenv("XAI_API_KEY") == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return grok.New()
}

func TestIntegration_TextToSpeech(t *testing.T) {
	c := client(t)
	audio, err := c.Voice.TextToSpeech(context.Background(), &voice.TTSRequest{
		Text:     "Hello",
		Language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(audio) == 0 {
		t.Fatal("expected non-empty audio bytes")
	}
}

func TestIntegration_ListVoices(t *testing.T) {
	c := client(t)
	voices, err := c.Voice.ListVoices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(voices) == 0 {
		t.Fatal("expected at least one voice")
	}
}
