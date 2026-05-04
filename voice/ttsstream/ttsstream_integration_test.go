package ttsstream_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/voice/ttsstream"
)

func apiKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return key
}

func TestIntegration_StreamTTS(t *testing.T) {
	key := apiKey(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := ttsstream.Dial(ctx, key, ttsstream.WithLanguage("en"), ttsstream.WithCodec("mp3"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.SendText(ctx, "Hello, world!"); err != nil {
		t.Fatal(err)
	}
	if err := s.Done(ctx); err != nil {
		t.Fatal(err)
	}

	var totalBytes int
	for chunk := range s.Audio() {
		totalBytes += len(chunk)
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if totalBytes == 0 {
		t.Fatal("expected non-zero audio bytes")
	}
	t.Logf("received %d bytes of audio", totalBytes)
}
