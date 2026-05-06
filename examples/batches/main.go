// batches demonstrates creating a batch job, adding requests, and iterating results.
//
// Batches run async at 50% cost. The workflow is:
//  1. Create an empty batch
//  2. Add chat completion requests
//  3. Wait until all requests are processed (or cancel)
//  4. Iterate results with AllResults
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/batches"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	// Create a batch.
	batch, err := client.Batches.Create(ctx, "example-batch")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(1)
	}
	fmt.Printf("created batch: %s\n", batch.BatchID)

	// Add requests.
	reqs := []batches.BatchRequest{
		{
			BatchRequestID: "req-1",
			BatchRequest: batches.BatchRequestPayload{
				ChatGetCompletion: map[string]any{
					"model":    "grok-4-1-fast-non-reasoning",
					"messages": []map[string]string{{"role": "user", "content": "What is 2+2?"}},
				},
			},
		},
		{
			BatchRequestID: "req-2",
			BatchRequest: batches.BatchRequestPayload{
				ChatGetCompletion: map[string]any{
					"model":    "grok-4-1-fast-non-reasoning",
					"messages": []map[string]string{{"role": "user", "content": "Boiling point of water in Celsius?"}},
				},
			},
		},
	}
	if err := client.Batches.AddRequests(ctx, batch.BatchID, reqs); err != nil {
		fmt.Fprintln(os.Stderr, "add requests:", err)
		os.Exit(1)
	}
	fmt.Printf("added %d requests\n", len(reqs))

	// Wait for the batch to finish processing (poll every 5 s).
	// Use a short context timeout so the example doesn't run forever.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	fmt.Println("waiting for batch to complete…")
	done, err := client.Batches.Wait(waitCtx, batch.BatchID, 5*time.Second)
	if err != nil {
		// If the context times out, cancel instead of leaving it running.
		if _, cerr := client.Batches.Cancel(ctx, batch.BatchID); cerr != nil {
			fmt.Fprintln(os.Stderr, "cancel:", cerr)
		}
		fmt.Fprintln(os.Stderr, "wait:", err)
		os.Exit(1)
	}
	fmt.Printf("batch done: success=%d error=%d\n", done.State.NumSuccess, done.State.NumError)

	// Iterate all results, AllResults fetches pages automatically.
	fmt.Println("\nresults:")
	for result, err := range client.Batches.AllResults(ctx, batch.BatchID, nil) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "results:", err)
			os.Exit(1)
		}
		fmt.Printf("  [%s] %+v\n", result.BatchRequestID, result.BatchResult)
	}

	// Also iterate all batches to show the All iterator.
	fmt.Println("\nall batches:")
	for b, err := range client.Batches.All(ctx, nil) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "list:", err)
			os.Exit(1)
		}
		fmt.Printf("  %s  %s  pending=%d\n", b.BatchID, b.Name, b.State.NumPending)
	}
}
