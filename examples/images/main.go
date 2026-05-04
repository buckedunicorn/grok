// images demonstrates image generation and saves the result URL.
package main

import (
	"context"
	"fmt"
	"os"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/images"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	n := 1
	resp, err := client.Images.Generate(context.Background(), &images.GenerateRequest{
		Model:  "grok-imagine-image",
		Prompt: "A photorealistic mountain lake at sunrise, reflections in still water",
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
