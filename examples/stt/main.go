// stt transcribes an audio file passed as the first argument.
//
// Usage:
//
//	XAI_API_KEY=... go run ./examples/stt /path/to/audio.mp3
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/voice"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: stt <audio-file>")
		os.Exit(1)
	}

	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	transcript, err := client.Voice.Transcribe(context.Background(), &voice.STTRequest{
		File:     f,
		Filename: os.Args[1],
		Language: "en",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(transcript.Text)
	fmt.Printf("\nduration: %.2fs  language: %s\n", transcript.Duration, transcript.Language)
}
