package agents_test

import (
	"context"
	"fmt"
	"log"

	"github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/agents"
)

func ExampleRunner_Run() {
	client := grok.New()
	runner := &agents.Runner{Client: client.Chat}

	agent := &agents.Agent{
		Name:         "assistant",
		Instructions: "You are a concise, helpful assistant.",
		Model:        "grok-3-mini-fast",
	}

	result, err := runner.Run(context.Background(), agent, agents.RunOptions{
		Input: "What is the capital of France?",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Output)
}

func ExampleRunner_Run_withTools() {
	client := grok.New()
	runner := &agents.Runner{Client: client.Chat}

	agent := &agents.Agent{
		Name:         "weather-agent",
		Instructions: "Answer questions using the available tools.",
		Model:        "grok-3-mini-fast",
		Tools: []agents.Tool{
			{
				Name:        "get_weather",
				Description: "Returns weather for a given city.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
					"required": []string{"city"},
				},
				Handler: func(_ context.Context, _ string) (string, error) {
					return `{"temp": "22°C", "condition": "sunny"}`, nil
				},
			},
		},
	}

	result, err := runner.Run(context.Background(), agent, agents.RunOptions{
		Input: "What is the weather in Paris?",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Output)
}
