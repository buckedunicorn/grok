// video-from-image animates a still image into a video (image-to-video).
//
// Pass a publicly-accessible image URL as the first argument.
//
// Usage:
//
//	XAI_API_KEY=... go run ./examples/video-from-image <image-url>
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
		fmt.Fprintln(os.Stderr, "usage: video-from-image <image-url>")
		os.Exit(1)
	}
	imageURL := os.Args[1]

	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	duration := 5
	requestID, err := client.Videos.Generate(ctx, &videos.GenerateRequest{
		Model:    "grok-imagine-video",
		Prompt:   "Bring this image to life with gentle, natural motion",
		Image:    &videos.VideoSource{URL: imageURL},
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
}
