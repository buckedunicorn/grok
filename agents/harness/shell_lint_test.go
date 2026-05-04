package harness_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/agents/harness"
	"github.com/buckedunicorn/grok/chat"
)

// runExecute drives one execute tool call through a stub-chat run so we
// exercise the same handler the model would invoke. The stub returns a
// tool_calls completion that asks for cmd/args/stdin, then a terminal
// completion so the run finishes cleanly.
func runExecute(t *testing.T, argsJSON string, stub *stubExecutor) string {
	t.Helper()
	c, cleanup := chatStub(t, []chat.Completion{
		toolCallCompletion("c1", "execute", argsJSON),
		textCompletion("done"),
	})
	defer cleanup()

	if stub == nil {
		stub = &stubExecutor{}
	}
	h := harness.NewDefault(c, "test-model",
		harness.WithExecutor(stub),
		harness.WithSubagent(false),
		harness.WithFS(nil),
		harness.WithPlanning(false),
	)
	res, err := h.Run(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(res.Trajectory.Turns[0].ToolCalls); got != 1 {
		t.Fatalf("expected 1 tool call, got %d", got)
	}
	return res.Trajectory.Turns[0].ToolCalls[0].Result
}

func TestExecute_lintsCmdWithSpaces(t *testing.T) {
	stub := &stubExecutor{}
	result := runExecute(t, `{"cmd":"python3 solution.py"}`, stub)
	if stub.called {
		t.Error("executor should NOT be called for cmd with whitespace")
	}
	var r map[string]any
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		t.Fatal(err)
	}
	errMsg, _ := r["error"].(string)
	if !strings.Contains(errMsg, "whitespace") {
		t.Errorf("expected whitespace hint in error, got %q", errMsg)
	}
}

func TestExecute_lintsEmptyCmd(t *testing.T) {
	stub := &stubExecutor{}
	result := runExecute(t, `{"cmd":""}`, stub)
	if stub.called {
		t.Error("executor should NOT be called for empty cmd")
	}
	var r map[string]any
	_ = json.Unmarshal([]byte(result), &r)
	if errMsg, _ := r["error"].(string); !strings.Contains(errMsg, "empty") {
		t.Errorf("expected empty-cmd hint, got %q", errMsg)
	}
}

func TestExecute_lintsBareInterpreter(t *testing.T) {
	cases := []string{"python", "python3", "node", "ruby", "bash", "sh", "zsh", "perl"}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			stub := &stubExecutor{}
			result := runExecute(t, `{"cmd":"`+cmd+`"}`, stub)
			if stub.called {
				t.Errorf("%s: executor must not be called for bare interpreter", cmd)
			}
			var r map[string]any
			_ = json.Unmarshal([]byte(result), &r)
			if errMsg, _ := r["error"].(string); !strings.Contains(errMsg, "REPL") {
				t.Errorf("%s: expected REPL hint, got %q", cmd, errMsg)
			}
		})
	}
}

func TestExecute_lintsBareInterpreterWithVersionSuffix(t *testing.T) {
	stub := &stubExecutor{}
	result := runExecute(t, `{"cmd":"python3.12"}`, stub)
	if stub.called {
		t.Error("python3.12 should also be linted as a bare interpreter")
	}
	var r map[string]any
	_ = json.Unmarshal([]byte(result), &r)
	if errMsg, _ := r["error"].(string); !strings.Contains(errMsg, "REPL") {
		t.Errorf("expected REPL hint for python3.12, got %q", errMsg)
	}
}

func TestExecute_allowsBareInterpreterWithStdin(t *testing.T) {
	// python3 with stdin is a legit way to run a script.
	stub := &stubExecutor{stdout: []byte("hi\n")}
	result := runExecute(t, `{"cmd":"python3","stdin":"print('hi')"}`, stub)
	if !stub.called {
		t.Error("python3 with stdin should be allowed")
	}
	if !strings.Contains(result, `"stdout":"hi\n"`) {
		t.Errorf("expected stdout in result, got %q", result)
	}
}

func TestExecute_allowsBareInterpreterWithArgs(t *testing.T) {
	stub := &stubExecutor{stdout: []byte("ok")}
	result := runExecute(t, `{"cmd":"python3","args":["solution.py"]}`, stub)
	if !stub.called {
		t.Error("python3 solution.py should run normally")
	}
	if !strings.Contains(result, `"stdout":"ok"`) {
		t.Errorf("expected stdout in result, got %q", result)
	}
}

func TestExecute_allowsArbitraryNonInterpreterWithoutArgs(t *testing.T) {
	// `ls` with no args is fine, it lists the cwd.
	stub := &stubExecutor{stdout: []byte("file1\nfile2\n")}
	result := runExecute(t, `{"cmd":"ls"}`, stub)
	if !stub.called {
		t.Error("ls with no args should run normally")
	}
	if !strings.Contains(result, "file1") {
		t.Errorf("expected ls output in result, got %q", result)
	}
}

func TestExecute_pathPrefixedInterpreterStillLinted(t *testing.T) {
	// /usr/bin/python3 is the same as python3, should still trip the lint.
	stub := &stubExecutor{}
	result := runExecute(t, `{"cmd":"/usr/bin/python3"}`, stub)
	if stub.called {
		t.Error("/usr/bin/python3 should be linted via filepath.Base")
	}
	var r map[string]any
	_ = json.Unmarshal([]byte(result), &r)
	if errMsg, _ := r["error"].(string); !strings.Contains(errMsg, "REPL") {
		t.Errorf("expected REPL hint, got %q", errMsg)
	}
}
