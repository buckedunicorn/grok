// models lists available language models with their pricing.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/buckedunicorn/grok"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))
	ctx := context.Background()

	models, err := client.Models.ListLanguage(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("%-40s  %s\n", "MODEL", "PROMPT $/M TOKENS")
	fmt.Println("─────────────────────────────────────────────────────────")
	for _, m := range models {
		price := " - "
		if m.PromptTextTokenPrice > 0 {
			price = fmt.Sprintf("$%.4f", float64(m.PromptTextTokenPrice)/1e12*1e6)
		}
		fmt.Printf("%-40s  %s\n", m.ID, price)
	}
}
