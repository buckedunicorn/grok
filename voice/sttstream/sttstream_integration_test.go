package sttstream_test

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/voice/sttstream"
)

func apiKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("XAI_API_KEY")
	if key == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return key
}

// silencePCM generates n samples of silence as 16-bit little-endian PCM.
func silencePCM(samples int) []byte {
	buf := make([]byte, samples*2)
	for i := range samples {
		// 440 Hz sine at low amplitude so the VAD can detect it as non-silence
		t := float64(i) / 16000.0
		v := int16(math.Sin(2*math.Pi*440*t) * 1000)
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
	}
	return buf
}

func TestIntegration_StreamSTT(t *testing.T) {
	key := apiKey(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s, err := sttstream.Dial(ctx, key,
		sttstream.WithSampleRate(16000),
		sttstream.WithEncoding("pcm"),
		sttstream.WithInterimResults(false),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	events := s.Listen(ctx)

	// Wait for ready before sending audio.
	select {
	case <-s.Ready():
	case <-ctx.Done():
		t.Fatal("timeout waiting for transcript.created")
	}

	// Send ~1 second of tone audio then signal done.
	audio := silencePCM(16000)
	if err := s.SendAudio(ctx, audio); err != nil {
		t.Fatal(err)
	}
	if err := s.Done(ctx); err != nil {
		t.Fatal(err)
	}

	// Drain events until transcript.done or timeout.
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			t.Logf("event: type=%s text=%q", ev.Type, ev.Text)
			if ev.Type == "transcript.done" {
				return
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for transcript.done")
		}
	}
}
