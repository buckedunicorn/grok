// tts synthesizes speech from text and writes the audio to a file.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/voice"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	audio, err := client.Voice.TextToSpeech(context.Background(), &voice.TTSRequest{
		Text:     "Hello from the xAI Grok Go SDK. This is a text-to-speech demonstration.",
		Language: "en",
		VoiceID:  "eve",
		OutputFormat: &voice.AudioFormat{
			Codec: "mp3",
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	out := "output.mp3"
	if err := os.WriteFile(out, audio, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d bytes to %s\n", len(audio), out)
}
