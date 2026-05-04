package files_test

import (
	"context"
	"os"
	"strings"
	"testing"

	grok "github.com/buckedunicorn/grok"
)

func client(t *testing.T) *grok.Client {
	t.Helper()
	if os.Getenv("XAI_API_KEY") == "" {
		t.Skip("XAI_API_KEY not set")
	}
	return grok.New()
}

func TestIntegration_UploadListDelete(t *testing.T) {
	c := client(t)
	content := `{"messages":[{"role":"user","content":"hello"}]}`
	f, err := c.Files.Upload(context.Background(), "test.jsonl", strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if f.ID == "" {
		t.Fatal("expected non-empty file ID")
	}

	list, err := c.Files.List(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range list.Data {
		if file.ID == f.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("uploaded file not found in list")
	}

	if err := c.Files.Delete(context.Background(), f.ID); err != nil {
		t.Fatal(err)
	}
}
