// image-edit modifies an existing image using a text prompt.
//
// Pass a publicly-accessible image URL as the first argument.
//
// Usage:
//
//	XAI_API_KEY=... go run ./examples/image-edit <image-url> [prompt]
package main

import (
	"context"
	"fmt"
	"os"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/images"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: image-edit <image-url> [prompt]")
		os.Exit(1)
	}
	imageURL := os.Args[1]
	prompt := "Add a dramatic sunset sky to the background"
	if len(os.Args) >= 3 {
		prompt = os.Args[2]
	}

	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	n := 1
	resp, err := client.Images.Edit(context.Background(), &images.EditRequest{
		Model:  "grok-imagine-image",
		Prompt: prompt,
		Image:  &images.ImageSource{URL: imageURL},
		N:      &n,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for i, img := range resp.Data {
		if img.URL != "" {
			fmt.Printf("image %d: %s\n", i+1, img.URL)
		} else {
			fmt.Printf("image %d: <b64_json, %d bytes>\n", i+1, len(img.B64JSON))
		}
	}
}
