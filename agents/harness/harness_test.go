package harness_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/agents/harness"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/internal/transport"
)

// chatStub returns a chat.Client wired to a httptest server whose handler
// returns the next queued completion on each call. The fixture is consumed
// in order.
func chatStub(t *testing.T, completions []chat.Completion) (*chat.Client, func()) {
	t.Helper()
	idx := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if idx >= len(completions) {
			t.Errorf("chatStub exhausted: requested call #%d but only %d queued", idx+1, len(completions))
			http.Error(w, "exhausted", 500)
			return
		}
		json.NewEncoder(w).Encode(completions[idx])
		idx++
	}))
	tr := transport.NewInsecure("test-key", srv.URL, srv.Client())
	return chat.NewClient(tr), srv.Close
}

func toolCallCompletion(toolCallID, name, args string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "tool_calls",
			Message: chat.Message{
				Role: "assistant",
				ToolCalls: []chat.ToolCall{{
					ID:       toolCallID,
					Type:     "function",
					Function: chat.FunctionCallData{Name: name, Arguments: args},
				}},
			},
		}},
	}
}

func textCompletion(text string) chat.Completion {
	return chat.Completion{
		Choices: []chat.Choice{{
			FinishReason: "stop",
			Message:      chat.Message{Role: "assistant", Content: text},
		}},
	}
}

func TestHarness_Run_writeAndReadFile(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		toolCallCompletion("c1", "write_file", `{"path":"hello.md","content":"world"}`),
		toolCallCompletion("c2", "read_file", `{"path":"hello.md"}`),
		textCompletion("file contents are: world"),
	})
	defer cleanup()

	h := harness.NewDefault(c, "test-model", harness.WithSubagent(false))
	res, err := h.Run(context.Background(), "Write 'world' to hello.md and read it back.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "world") {
		t.Errorf("output = %q", res.Output)
	}
	if got := len(res.Trajectory.Turns); got != 3 {
		t.Errorf("turns = %d, want 3", got)
	}
}

func TestHarness_Run_writeTodos(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		toolCallCompletion("c1", "write_todos", `{"items":[{"content":"step 1"},{"content":"step 2"}]}`),
		textCompletion("planned"),
	})
	defer cleanup()

	h := harness.NewDefault(c, "test-model", harness.WithSubagent(false))
	res, err := h.Run(context.Background(), "plan two steps")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Trajectory.Turns); got != 2 {
		t.Errorf("turns = %d, want 2", got)
	}
	tc := res.Trajectory.Turns[0].ToolCalls[0]
	if tc.Name != "write_todos" || !strings.Contains(tc.Result, `"count":2`) {
		t.Errorf("write_todos result = %+v", tc)
	}
}

func TestHarness_NoFS_disabled(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		toolCallCompletion("c1", "read_file", `{"path":"x"}`),
		textCompletion("done"),
	})
	defer cleanup()

	// Pass nil FS to disable the FS tools entirely.
	h := harness.NewDefault(c, "test-model",
		harness.WithFS(nil),
		harness.WithSubagent(false),
	)
	// The model will try to call read_file but since the tool is not
	// registered, the runner returns an error result. The trajectory should
	// record the unknown-tool error.
	res, err := h.Run(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	tc := res.Trajectory.Turns[0].ToolCalls[0]
	if tc.Err == nil {
		t.Error("expected unknown-tool error to be recorded")
	}
}

func TestHarness_Subagent_spawnsChildAndReturnsOutput(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		// Parent: invoke task tool with a child prompt.
		toolCallCompletion("c1", "task", `{"description":"summarize","prompt":"What is 2+2?"}`),
		// Child: produces a final answer directly (no tool calls).
		textCompletion("4"),
		// Parent: terminal turn.
		textCompletion("Subagent answered: 4"),
	})
	defer cleanup()

	h := harness.NewDefault(c, "test-model")
	res, err := h.Run(context.Background(), "delegate")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "4") {
		t.Errorf("parent output = %q", res.Output)
	}
	// Parent trajectory shows one tool call to "task" with result containing "final_output":"4"
	if got := len(res.Trajectory.Turns); got != 2 {
		t.Errorf("parent turns = %d, want 2", got)
	}
	tc := res.Trajectory.Turns[0].ToolCalls[0]
	if tc.Name != "task" {
		t.Errorf("expected task tool, got %q", tc.Name)
	}
	if !strings.Contains(tc.Result, `"final_output":"4"`) {
		t.Errorf("subagent result = %q", tc.Result)
	}
}

func TestHarness_Subagent_childCannotSpawnGrandchild(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		// Parent invokes task.
		toolCallCompletion("c1", "task", `{"description":"x","prompt":"go"}`),
		// Child tries to invoke task again, but task is not registered for children.
		toolCallCompletion("c2", "task", `{"description":"y","prompt":"deeper"}`),
		// Child sees unknown-tool error result and produces a final answer.
		textCompletion("done"),
		// Parent terminal turn.
		textCompletion("ok"),
	})
	defer cleanup()

	h := harness.NewDefault(c, "test-model")
	res, err := h.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	// In the parent's view we got back an "ok" answer. The child's grandchild
	// attempt should have been rejected as unknown_tool, verify by looking at
	// the parent's task result, which embeds final_output.
	if !strings.Contains(res.Output, "ok") {
		t.Errorf("parent output = %q", res.Output)
	}
}

func TestHarness_RequiresClient(t *testing.T) {
	h := &harness.Harness{Model: "x"}
	_, err := h.Run(context.Background(), "task")
	if err == nil {
		t.Error("expected error for nil Client")
	}
}

func TestHarness_RequiresModel(t *testing.T) {
	c, cleanup := chatStub(t, nil)
	defer cleanup()
	h := &harness.Harness{Client: c}
	_, err := h.Run(context.Background(), "task")
	if err == nil {
		t.Error("expected error for empty Model")
	}
}

func TestHarness_WithExecutor_enablesShellTool(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		toolCallCompletion("c1", "execute", `{"cmd":"stub","args":["a","b"]}`),
		textCompletion("done"),
	})
	defer cleanup()

	stub := &stubExecutor{stdout: []byte("ok")}
	h := harness.NewDefault(c, "test-model",
		harness.WithExecutor(stub),
		harness.WithSubagent(false),
	)
	res, err := h.Run(context.Background(), "run a thing")
	if err != nil {
		t.Fatal(err)
	}
	if !stub.called {
		t.Error("stub executor was not called")
	}
	tc := res.Trajectory.Turns[0].ToolCalls[0]
	if tc.Name != "execute" || !strings.Contains(tc.Result, `"stdout":"ok"`) {
		t.Errorf("execute result = %+v", tc)
	}
}

type stubExecutor struct {
	called bool
	stdout []byte
	stderr []byte
}

func (s *stubExecutor) Run(_ context.Context, _ string, _ []string, _ []byte) ([]byte, []byte, error) {
	s.called = true
	return s.stdout, s.stderr, nil
}

// --- escape-blunder normalization tests (write_file + edit_file) ---

func TestHarness_WriteFile_normalizesLiteralBackslashN(t *testing.T) {
	c, cleanup := chatStub(t, []chat.Completion{
		// Model emits the common mistake: literal "\n" in JSON string.
		// JSON-decode produces Go string "line1\nline2" with literal backslash-n.
		toolCallCompletion("c1", "write_file", `{"path":"poem.txt","content":"line1\\nline2\\nline3"}`),
		textCompletion("done"),
	})
	defer cleanup()

	fs := harness.NewMemoryFS()
	h := harness.NewDefault(c, "test-model",
		harness.WithFS(fs),
		harness.WithSubagent(false),
	)
	if _, err := h.Run(context.Background(), "write a poem"); err != nil {
		t.Fatal(err)
	}

	got, err := fs.ReadFile("poem.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := "line1\nline2\nline3"
	if got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

func TestHarness_WriteFile_preservesGenuineLiteralBackslashN(t *testing.T) {
	// Model writes content that legitimately contains "\n" text, for
	// example, a Go source line with a regex pattern. The presence of a
	// real newline elsewhere in the content signals the model knew what it
	// was doing, so we must NOT normalize.
	c, cleanup := chatStub(t, []chat.Completion{
		toolCallCompletion("c1", "write_file",
			`{"path":"main.go","content":"package main\nvar pat = `+"`"+`\\n`+"`"+`\nfunc main() {}"}`),
		textCompletion("done"),
	})
	defer cleanup()

	fs := harness.NewMemoryFS()
	h := harness.NewDefault(c, "test-model",
		harness.WithFS(fs),
		harness.WithSubagent(false),
	)
	if _, err := h.Run(context.Background(), "write code"); err != nil {
		t.Fatal(err)
	}

	got, _ := fs.ReadFile("main.go")
	// Must still contain the literal backslash-n inside the regex backtick.
	if !strings.Contains(got, `\n`) {
		t.Errorf("genuine literal \\n was mangled: %q", got)
	}
	// And must have real newlines from the JSON \n escapes.
	if !strings.Contains(got, "\n") {
		t.Errorf("real newlines missing: %q", got)
	}
}

func TestHarness_WriteFile_leavesAlreadyCorrectContent(t *testing.T) {
	// Model gets escaping right: JSON args with proper "\n" escape that
	// decodes to real newline. Output should be unchanged.
	c, cleanup := chatStub(t, []chat.Completion{
		toolCallCompletion("c1", "write_file", `{"path":"good.txt","content":"a\nb"}`),
		textCompletion("done"),
	})
	defer cleanup()

	fs := harness.NewMemoryFS()
	h := harness.NewDefault(c, "test-model",
		harness.WithFS(fs),
		harness.WithSubagent(false),
	)
	if _, err := h.Run(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	got, _ := fs.ReadFile("good.txt")
	if got != "a\nb" {
		t.Errorf("file = %q, want %q", got, "a\nb")
	}
}
