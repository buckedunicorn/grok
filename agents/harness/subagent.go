package harness

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/buckedunicorn/grok/agents"
)

// subagentTool produces the "task" tool that spawns an isolated child run.
//
// Each subagent invocation creates a fresh Harness configured the same as
// the parent EXCEPT:
//   - subagent recursion is disabled (subagentDepth = -1) so children
//     cannot spawn grandchildren
//   - the child gets a brand-new in-memory Session, no message history
//     leaks between parent and child or across sibling subagent calls
//
// The subagent's final answer is returned to the parent as the tool result.
// Trajectory of the child run is currently NOT propagated upward; if/when
// callers need it, the harness can be extended to attach child trajectories
// to a parent ToolCallTrace.
func subagentTool(parent *Harness) agents.Tool {
	return agents.Tool{
		Name: "task",
		Description: "Delegate a focused subtask to an isolated child agent. The child gets a fresh " +
			"context window and the same toolset (minus this 'task' tool). Use this to keep " +
			"the parent's context lean for long-horizon work. Returns the child's final answer.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"description": map[string]any{
					"type":        "string",
					"description": "Short label describing what the subtask does. Recorded in the trajectory.",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The complete task prompt for the child agent.",
				},
			},
			"required": []string{"prompt"},
		},
		Handler: func(ctx context.Context, args string) (string, error) {
			var p struct {
				Description string `json:"description"`
				Prompt      string `json:"prompt"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("task: invalid args: %w", err)
			}
			if p.Prompt == "" {
				return "", fmt.Errorf("task: prompt is required")
			}

			// Build a child Harness mirroring the parent but with subagents
			// forbidden in the child. Tools, FS, executor, instructions are
			// inherited; the child gets a fresh State (and thus a fresh
			// in-memory FS only if FS is the per-run default, note: the
			// parent's installed FS is shared. If callers want strict
			// per-subagent FS isolation, they should install a per-call FS
			// at the parent level.).
			child := &Harness{
				Client:         parent.Client,
				Model:          parent.Model,
				Instructions:   parent.Instructions,
				MaxTurns:       parent.MaxTurns,
				Hooks:          parent.Hooks,
				Middleware:     parent.Middleware,
				enablePlanning: parent.enablePlanning,
				fs:             parent.fs,
				executor:       parent.executor,
				enableSubagent: false, // children cannot spawn grandchildren
				subagentDepth:  -1,
			}

			result, err := child.Run(ctx, p.Prompt)
			if err != nil {
				out, _ := json.Marshal(map[string]any{
					"description": p.Description,
					"error":       err.Error(),
				})
				return string(out), err
			}
			out, _ := json.Marshal(map[string]any{
				"description":  p.Description,
				"final_output": result.Output,
				"turns":        len(result.Trajectory.Turns),
			})
			return string(out), nil
		},
	}
}
