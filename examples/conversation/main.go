// conversation demonstrates multi-turn chat using chat.Conversation,
// which automatically tracks message history across turns.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	cv := chat.NewConversation(client.Chat, "grok-4-1-fast-reasoning",
		chat.WithSystem("You are a concise assistant. Keep answers to one sentence."),
	)

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Multi-turn chat (ctrl+c or ctrl+d to quit):")
	for {
		fmt.Print("\nYou: ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		comp, err := cv.Send(context.Background(), line)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		if len(comp.Choices) > 0 {
			fmt.Println("Grok:", comp.Choices[0].Message.Content)
		}
	}

	fmt.Printf("\n\n--- history (%d messages) ---\n", len(cv.Messages()))
	for _, m := range cv.Messages() {
		fmt.Printf("[%s] %v\n", m.Role, m.Content)
	}
}
