package grok_test

import (
	"os"

	"github.com/buckedunicorn/grok"
)

func ExampleNew() {
	// New reads XAI_API_KEY from the environment when WithAPIKey is omitted.
	client := grok.New()
	_ = client
}

func ExampleNew_options() {
	client := grok.New(
		grok.WithAPIKey(os.Getenv("XAI_API_KEY")),
		grok.WithMaxRetries(3),
		grok.WithConvID("session-42"),
	)
	_ = client
}

func ExampleClient_WithConvID() {
	client := grok.New()

	// Scope a copy of the client to a specific conversation for cache locality.
	scoped := client.WithConvID("conv-abc")
	_ = scoped
}
