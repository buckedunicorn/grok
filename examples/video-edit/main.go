// video-edit edits an existing video using a text prompt.
//
// Pass a publicly-accessible MP4 URL as the first argument.
//
// Usage:
//
//	XAI_API_KEY=... go run ./examples/video-edit <video-url> [prompt]
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/videos"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: video-edit <video-url> [prompt]")
		os.Exit(1)
	}
	videoURL := os.Args[1]
	prompt := "Make the scene look like it was shot at night"
	if len(os.Args) >= 3 {
		prompt = os.Args[2]
	}

	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	requestID, err := client.Videos.Edit(ctx, &videos.EditRequest{
		Model:  "grok-imagine-video",
		Prompt: prompt,
		Video:  videos.VideoSource{URL: videoURL},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("submitted: request_id=%s\npolling", requestID)

	result, err := client.Videos.Wait(ctx, requestID, 5*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "\n"+err.Error())
		os.Exit(1)
	}
	fmt.Println()
	fmt.Printf("done: %s\n", result.Video.URL)
}
