package images_test

import (
	"context"
	"os"
	"testing"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/images"
)

func client(t *testing.T) *grok.Client {
	t.Helper()
	if os.Getenv("XAI_API_KEY") == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return grok.New()
}

func TestIntegration_Generate(t *testing.T) {
	c := client(t)
	resp, err := c.Images.Generate(context.Background(), &images.GenerateRequest{
		Model:  "grok-imagine-image",
		Prompt: "a simple red circle on white background",
		N:      intPtr(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("expected at least one image")
	}
	if resp.Data[0].URL == "" && resp.Data[0].B64JSON == "" {
		t.Fatal("expected URL or b64_json in response")
	}
}

func intPtr(v int) *int { return &v }
