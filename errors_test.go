package grok_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/internal/apierr"
)

func TestAPIErrorAs(t *testing.T) {
	// Simulate a transport error bubbling up
	inner := &apierr.APIError{StatusCode: 429, Code: "rate_limit", Message: "too many requests"}
	err := fmt.Errorf("chat.Create: %w", inner)

	var apiErr *grok.APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("errors.As(*grok.APIError) did not match")
	}
	if apiErr.StatusCode != 429 {
		t.Fatalf("StatusCode: got %d, want 429", apiErr.StatusCode)
	}
}
