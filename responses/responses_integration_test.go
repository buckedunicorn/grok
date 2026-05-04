package responses_test

import (
	"context"
	"os"
	"testing"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/responses"
)

func client(t *testing.T) *grok.Client {
	t.Helper()
	if os.Getenv("XAI_API_KEY") == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return grok.New()
}

func TestIntegration_Create(t *testing.T) {
	c := client(t)
	resp, err := c.Responses.Create(context.Background(), &responses.CreateRequest{
		Model: "grok-3-mini-fast",
		Input: "Reply with one word: hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.OutputText() == "" {
		t.Fatal("expected non-empty output text")
	}
}

func TestIntegration_GetDelete(t *testing.T) {
	c := client(t)
	resp, err := c.Responses.Create(context.Background(), &responses.CreateRequest{
		Model: "grok-3-mini-fast",
		Input: "Reply with one word: hello",
		Store: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Responses.Get(context.Background(), resp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != resp.ID {
		t.Fatalf("got id %q, want %q", got.ID, resp.ID)
	}
	if err := c.Responses.Delete(context.Background(), resp.ID); err != nil {
		t.Fatal(err)
	}
}

func boolPtr(v bool) *bool { return &v }
