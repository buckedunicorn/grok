// video-extend continues an existing video for additional seconds.
//
// Pass a publicly-accessible MP4 URL as the first argument.
//
// Usage:
//
//	XAI_API_KEY=... go run ./examples/video-extend <video-url> [prompt]
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/videos"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: video-extend <video-url> [prompt]")
		os.Exit(1)
	}
	videoURL := os.Args[1]
	prompt := "Continue the scene with a slow camera pan to the right"
	if len(os.Args) >= 3 {
		prompt = os.Args[2]
	}

	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	duration := 6
	requestID, err := client.Videos.Extend(ctx, &videos.ExtendRequest{
		Model:    "grok-imagine-video",
		Prompt:   prompt,
		Video:    videos.VideoSource{URL: videoURL},
		Duration: &duration,
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
	fmt.Printf("done: %s  (%ds)\n", result.Video.URL, result.Video.Duration)
}
