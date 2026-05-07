package responses_test

import (
	"context"
	"fmt"
	"log"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/responses"
)

func ExampleClient_Create() {
	client := grok.New()
	resp, err := client.Responses.Create(context.Background(), &responses.CreateRequest{
		Model: "grok-3-mini-fast",
		Input: "Explain Go interfaces in one sentence.",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp.OutputText())
}

func ExampleClient_Create_chained() {
	client := grok.New()

	// First turn.
	store := true
	resp, err := client.Responses.Create(context.Background(), &responses.CreateRequest{
		Model: "grok-3-mini-fast",
		Input: "What is the Go programming language?",
		Store: &store,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Chain the next turn via PreviousResponseID — no need to resend history.
	resp2, err := client.Responses.Create(context.Background(), &responses.CreateRequest{
		Model:              "grok-3-mini-fast",
		Input:              "Who designed it?",
		PreviousResponseID: resp.ID,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(resp2.OutputText())
}
