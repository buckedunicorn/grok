package batches_test

import (
	"context"
	"os"
	"testing"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/batches"
)

func client(t *testing.T) *grok.Client {
	t.Helper()
	if os.Getenv("XAI_API_KEY") == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return grok.New()
}

func TestIntegration_CreateListCancel(t *testing.T) {
	c := client(t)
	batch, err := c.Batches.Create(context.Background(), "test-batch")
	if err != nil {
		t.Fatal(err)
	}
	if batch.BatchID == "" {
		t.Fatal("expected non-empty batch ID")
	}

	list, err := c.Batches.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range list.Batches {
		if b.BatchID == batch.BatchID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("created batch not found in list")
	}

	err = c.Batches.AddRequests(context.Background(), batch.BatchID, []batches.BatchRequest{
		{
			BatchRequestID: "req-1",
			BatchRequest: batches.BatchRequestPayload{
				ChatGetCompletion: map[string]any{
					"model": "grok-3-mini-fast",
					"messages": []map[string]string{
						{"role": "user", "content": "Say hello"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := c.Batches.Cancel(context.Background(), batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.BatchID != batch.BatchID {
		t.Fatalf("cancelled batch ID mismatch: got %q, want %q", cancelled.BatchID, batch.BatchID)
	}
}
