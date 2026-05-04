package harness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/buckedunicorn/grok/agents"
)

// planningTools returns write_todos and list_todos bound to state.
func planningTools(state *State) []agents.Tool {
	return []agents.Tool{
		writeTodosTool(state),
		listTodosTool(state),
	}
}

func writeTodosTool(state *State) agents.Tool {
	return agents.Tool{
		Name: "write_todos",
		Description: "Replace the todo list with a new set of items. Use this to plan " +
			"a multi-step task before executing or to update progress after each step.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type":        "array",
					"description": "Ordered list of todo items.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"content": map[string]any{
								"type":        "string",
								"description": "Short description of the step.",
							},
							"status": map[string]any{
								"type":        "string",
								"description": "One of: pending, in_progress, done.",
								"enum":        []string{"pending", "in_progress", "done"},
							},
						},
						"required": []string{"content"},
					},
				},
			},
			"required": []string{"items"},
		},
		Handler: func(_ context.Context, argsJSON string) (string, error) {
			var args struct {
				Items []Todo `json:"items"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
				return "", fmt.Errorf("write_todos: invalid args: %w", err)
			}
			state.SetTodos(args.Items)

			out, _ := json.Marshal(map[string]any{
				"ok":    true,
				"count": len(args.Items),
				"todos": state.Todos(),
			})
			return string(out), nil
		},
	}
}

func listTodosTool(state *State) agents.Tool {
	return agents.Tool{
		Name:        "list_todos",
		Description: "Return the current todo list with statuses.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ string) (string, error) {
			todos := state.Todos()
			out, _ := json.Marshal(map[string]any{"todos": todos})
			return string(out), nil
		},
	}
}
