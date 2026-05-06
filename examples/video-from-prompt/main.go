// video-from-prompt generates a video from a text prompt and polls until complete.
//
// Video generation is async: create returns a request_id, then poll until done.
//
// Usage:
//
//	XAI_API_KEY=... go run ./examples/video-from-prompt
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
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	duration := 5
	requestID, err := client.Videos.Generate(ctx, &videos.GenerateRequest{
		Model:    "grok-imagine-video",
		Prompt:   "A time-lapse of clouds rolling over a mountain range at golden hour",
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
	fmt.Printf("done: %s\n", result.Video.URL)
	fmt.Printf("duration: %ds\n", result.Video.Duration)
}
