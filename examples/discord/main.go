// discord demonstrates the discord.Router, a zero-dependency adapter that
// manages per-channel chat.Conversations and formats replies for Discord's
// 2000-character message limit.
//
// Wire Router.Handle into whichever Discord library you use (e.g. discordgo,
// disgo, arikawa) in place of the stub below.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/discord"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	router := discord.NewRouter(client.Chat, discord.RouterConfig{
		Model:      "grok-4-1-fast-non-reasoning",
		System:     "You are a concise and friendly Discord assistant.",
		MaxHistory: 10, // keep the last 10 turn-pairs per channel
	})

	// Simulate messages arriving in two different channels.
	type event struct {
		channelID string
		userMsg   string
	}

	events := []event{
		{"ch-general", "What is Go good at?"},
		{"ch-random", "Tell me a one-line joke."},
		{"ch-general", "And what are its weaknesses?"},
	}

	for _, e := range events {
		reply, err := router.Handle(context.Background(), e.channelID, e.userMsg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] error: %v\n", e.channelID, err)
			continue
		}

		// In a real bot you'd send each chunk as a separate Discord message.
		chunks := discord.Chunks(reply, 2000)
		fmt.Printf("[%s] user: %s\n", e.channelID, e.userMsg)
		for i, chunk := range chunks {
			label := "bot"
			if len(chunks) > 1 {
				label = fmt.Sprintf("bot (part %d/%d)", i+1, len(chunks))
			}
			fmt.Printf("[%s] %s: %s\n\n", e.channelID, label, chunk)
		}
	}

	// Reset a channel's history (e.g. on /reset slash command).
	router.Reset("ch-general")
	fmt.Println("ch-general history cleared.")

	// Demonstrate long-message chunking.
	longText := strings.Repeat("word ", 1000)
	chunks := discord.Chunks(longText, 2000)
	fmt.Printf("\nLong message split into %d chunks.\n", len(chunks))
}
