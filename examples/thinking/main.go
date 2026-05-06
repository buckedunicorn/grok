// thinking demonstrates chat.ThinkingContent and chat.StripThinking.
//
// xAI returns reasoning in two possible places, and ThinkingContent
// transparently checks both:
//
//  1. The dedicated `reasoning_content` field on the assistant message  -
//     used by current xAI reasoning models. Surfaced via
//     chat.Message.ReasoningContent.
//  2. Inline `<think>...</think>` tags inside the message content  -
//     used by some older or simpler models that bake reasoning into the
//     visible response text.
//
// Model choice still matters for the reasoning_effort knob: the
// grok-3-mini family accepts ReasoningEffort ("low" | "high"). The
// grok-4-1-fast-reasoning family reasons intrinsically and rejects
// reasoning_effort as an invalid argument.
//
// This example uses grok-3-mini-fast so reasoning_effort works. Drop
// the ReasoningEffort line if you want to try grok-4-1-fast-reasoning  -
// reasoning will still appear in ReasoningContent.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	comp, err := client.Chat.Create(context.Background(), &chat.CreateRequest{
		// grok-3-mini-fast supports reasoning_effort and emits visible <think> blocks.
		Model:           "grok-3-mini-fast",
		ReasoningEffort: "low",
		Messages: []chat.Message{
			{Role: "user", Content: "In one sentence, what is the fastest sorting algorithm for nearly-sorted data?"},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Extract the hidden reasoning, if any.
	thinking := chat.ThinkingContent(comp)
	if thinking != "" {
		fmt.Println("=== Thinking ===")
		fmt.Println(thinking)
		fmt.Println()
	}

	// Show only the final answer.
	clean := chat.StripThinking(comp)
	fmt.Println("=== Answer ===")
	fmt.Println(clean.Choices[0].Message.Content)

	fmt.Printf("\nReasoning tokens: %d\n", comp.Usage.CompletionTokensDetails.ReasoningTokens)
}
