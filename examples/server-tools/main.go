// server-tools demonstrates xAI's server-side tools on the Responses API.
//
// Unlike client-side function calling, where you supply a Go handler and
// the SDK dispatches tool calls, server-side tools execute on xAI's
// infrastructure. You enable them by adding type-only entries to the tools
// array; there is no handler to write. Results stream back inline as
// part of the Response.Output items.
//
// Available tools (as of late 2026):
//
//   - web_search         , current web results
//   - x_search           , search posts on X
//   - code_interpreter   , Python in a sandboxed runtime
//   - collections_search , query xAI-managed document collections
//
// Cost: every invocation counts. Response.Usage.NumServerSideToolsUsed
// reports how many fired in a single turn.
//
// Endpoint constraint: server-side tools are Responses API only, the
// /v1/chat/completions endpoint does NOT accept these tool types.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/responses"
)

func main() {
	tool := flag.String("tool", "web_search",
		"server-side tool to demo: web_search | x_search | code_interpreter | collections_search")
	flag.Parse()

	prompt := defaultPromptFor(*tool)
	if flag.NArg() > 0 {
		prompt = flag.Arg(0)
	}

	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	resp, err := client.Responses.Create(context.Background(), &responses.CreateRequest{
		Model: "grok-4-1-fast-reasoning",
		Input: prompt,
		Tools: []responses.Tool{
			{Type: *tool},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("=== Tool: %s ===\n", *tool)
	fmt.Printf("Prompt: %s\n\n", prompt)

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			fmt.Println("--- assistant message ---")
			fmt.Println(item.OutputText())
			fmt.Println()
		default:
			// Reasoning items, tool-invocation traces, citations, etc.
			fmt.Printf("--- %s item (id=%s, status=%s) ---\n", item.Type, item.ID, item.Status)
		}
	}

	if resp.Usage != nil {
		fmt.Printf("Usage: input=%d output=%d total=%d server_tools_used=%d\n",
			resp.Usage.InputTokens,
			resp.Usage.OutputTokens,
			resp.Usage.TotalTokens,
			resp.Usage.NumServerSideToolsUsed,
		)
	}
}

func defaultPromptFor(tool string) string {
	switch tool {
	case "x_search":
		return "What are people saying about Go 1.26 on X this week?"
	case "code_interpreter":
		return "Compute the 100th Fibonacci number using Python."
	case "collections_search":
		return "Search the connected collection for any documents mentioning grok."
	default: // web_search
		return "What's the latest stable Go release and its key changes?"
	}
}
