package models_test

import (
	"context"
	"os"
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

func TestIntegration_List(t *testing.T) {
	c := client(t)
	models, err := c.Models.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one model")
	}
}

func TestIntegration_ListLanguage(t *testing.T) {
	c := client(t)
	models, err := c.Models.ListLanguage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) == 0 {
		t.Fatal("expected at least one language model")
	}
}
