// responses demonstrates the stateful Responses API with multi-turn chaining.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/responses"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	// Turn 1
	resp1, err := client.Responses.Create(ctx, &responses.CreateRequest{
		Model: "grok-4-1-fast-non-reasoning",
		Input: "My name is Alice. Remember that.",
		Store: boolPtr(true),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Turn 1:", resp1.OutputText())

	// Turn 2, chained via PreviousResponseID, no need to resend history.
	resp2, err := client.Responses.Create(ctx, &responses.CreateRequest{
		Model:              "grok-4-1-fast-non-reasoning",
		Input:              "What is my name?",
		PreviousResponseID: resp1.ID,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Turn 2:", resp2.OutputText())

	// Clean up stored responses.
	_ = client.Responses.Delete(ctx, resp1.ID)
	_ = client.Responses.Delete(ctx, resp2.ID)
}

func boolPtr(v bool) *bool { return &v }
