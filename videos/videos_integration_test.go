package videos_test

import (
	"context"
	"os"
	"testing"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/videos"
)

func client(t *testing.T) *grok.Client {
	t.Helper()
	if os.Getenv("XAI_API_KEY") == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return grok.New()
}

func TestIntegration_GenerateAndPoll(t *testing.T) {
	c := client(t)
	requestID, err := c.Videos.Generate(context.Background(), &videos.GenerateRequest{
		Model:  "grok-imagine-video",
		Prompt: "a single red dot moving across a white background",
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestID == "" {
		t.Fatal("expected non-empty request_id")
	}

	// Poll once, we only verify the API accepted the job; waiting for done is too slow for CI.
	result, err := c.Videos.GetResult(context.Background(), requestID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == "" {
		t.Fatal("expected non-empty status")
	}
	t.Logf("video generation status: %s (progress %d%%)", result.Status, result.Progress)
}
