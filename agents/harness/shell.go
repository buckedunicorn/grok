package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/buckedunicorn/grok/agents"
)

// Executor runs a single command on behalf of the shell tool.
//
// LocalExecutor is the default implementation and runs commands directly on
// the host with no sandbox, appropriate for local-developer workflows. For
// production agentic workloads, plug in a Docker, Firecracker, gVisor, or
// other sandboxing Executor before enabling the shell tool.
//
// Implementations must respect ctx cancellation. Returned stdout and stderr
// are byte slices so callers can detect non-UTF8 output; the shell tool
// converts them to strings for the model.
type Executor interface {
	Run(ctx context.Context, cmd string, args []string, stdin []byte) (stdout, stderr []byte, err error)
}

// LocalExecutor runs commands via os/exec with no sandbox. The agent has
// the same authority as the parent process. Use deliberately.
type LocalExecutor struct {
	// Workdir is the directory commands run in. Empty means inherit.
	Workdir string
	// Env, when non-nil, replaces the inherited environment. Format: "KEY=VALUE".
	Env []string
}

// Run invokes cmd with args, optional stdin, and the configured Workdir/Env.
func (l *LocalExecutor) Run(ctx context.Context, cmd string, args []string, stdin []byte) ([]byte, []byte, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	if l.Workdir != "" {
		c.Dir = l.Workdir
	}
	if l.Env != nil {
		c.Env = l.Env
	}
	if len(stdin) > 0 {
		c.Stdin = bytes.NewReader(stdin)
	}
	var out, errBuf bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errBuf
	err := c.Run()
	return out.Bytes(), errBuf.Bytes(), err
}

func shellTool(e Executor) agents.Tool {
	return agents.Tool{
		Name: "execute",
		Description: `Run an executable in the sandbox. Returns stdout, stderr, and exit_code.
Example: {"cmd":"python3","args":["solution.py"]} runs ` + "`python3 solution.py`" + `.
cmd is the program name (no spaces). args is the argument list (pass [] if none). Prefer FS tools for file operations.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cmd": map[string]any{
					"type":        "string",
					"description": `Executable name (e.g. "python3", "ls", "go"). No spaces, no arguments.`,
				},
				"args": map[string]any{
					"type":        "array",
					"description": `Arguments passed to cmd, in order. Pass [] for no arguments. e.g. ["solution.py"] or ["-la","/workspace"].`,
					"items":       map[string]any{"type": "string"},
				},
				"stdin": map[string]any{
					"type":        "string",
					"description": "Optional stdin content piped to the process.",
				},
			},
			// args is required so the JSON Schema validator forces the
			// model to specify it explicitly. Empty list is valid;
			// absent is not. Without this, reasoning models routinely
			// call e.g. {"cmd":"python3"} and silently land in a REPL
			// with no output.
			"required": []string{"cmd", "args"},
		},
		Handler: func(ctx context.Context, args string) (string, error) {
			var p struct {
				Cmd   string   `json:"cmd"`
				Args  []string `json:"args"`
				Stdin string   `json:"stdin"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", fmt.Errorf("execute: invalid args: %w", err)
			}
			if hint := lintExecuteArgs(p.Cmd, p.Args, p.Stdin); hint != "" {
				// Refuse to dispatch, return a tool result the model can
				// learn from instead of letting it loop on a silent
				// success. Surfaced as an explicit error field so the
				// model recognizes it as a tool-call problem, not a
				// program failure.
				out, _ := json.Marshal(map[string]any{
					"error":     hint,
					"exit_code": -1,
					"stdout":    "",
					"stderr":    "",
				})
				return string(out), nil
			}
			stdout, stderr, runErr := e.Run(ctx, p.Cmd, p.Args, []byte(p.Stdin))
			result := map[string]any{
				"stdout": string(stdout),
				"stderr": string(stderr),
			}
			if runErr != nil {
				result["error"] = runErr.Error()
				if exitErr, ok := runErr.(*exec.ExitError); ok {
					result["exit_code"] = exitErr.ExitCode()
				}
			} else {
				result["exit_code"] = 0
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		},
	}
}

// silentReplInterpreters is the set of cmd values that, when invoked with
// no args and no stdin, drop into a REPL or wait for stdin and exit
// cleanly with empty output, silently misleading any agent that's trying
// to verify a result.
var silentReplInterpreters = map[string]struct{}{
	"python":  {},
	"python3": {},
	"node":    {},
	"deno":    {},
	"bun":     {},
	"ruby":    {},
	"irb":     {},
	"perl":    {},
	"sh":      {},
	"bash":    {},
	"zsh":     {},
	"dash":    {},
	"fish":    {},
	"ash":     {},
	"php":     {},
}

// lintExecuteArgs returns a non-empty hint when the cmd/args combination is
// almost certainly a model mistake. Returning "" means "looks fine,
// dispatch to the executor."
//
// Two patterns we catch and refuse:
//
// 1. cmd contains spaces (e.g. "python3 solution.py"). Models sometimes
// pass a whole command line in cmd; the executor would then try to
// find a binary literally named "python3 solution.py". Better to
// refuse with a clear hint than to fail with "executable not found"
// from os/exec.
//
// 2. cmd is a known REPL interpreter and both args and stdin are empty.
// Running e.g. python3 with nothing produces a REPL that immediately
// EOFs and exits clean with no output, exit_code 0 / stdout empty /
// stderr empty. Indistinguishable from a script that ran but didn't
// print, so the model loops trying to "see" results.
func lintExecuteArgs(cmd string, args []string, stdin string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "execute: cmd is empty. Pass the executable name in cmd and any arguments in args."
	}
	if strings.ContainsAny(cmd, " \t") {
		return fmt.Sprintf(`execute: cmd %q contains whitespace. cmd is the executable name only, pass arguments via the args array. e.g. {"cmd":"python3","args":["solution.py"]}`, cmd)
	}

	base := filepath.Base(cmd)
	// Strip a trailing ".N" / ".N.M" version suffix so e.g. "python3.12"
	// matches "python3". Conservative: only strips numeric segments.
	base = stripTrailingVersion(base)

	if _, hit := silentReplInterpreters[base]; hit && len(args) == 0 && stdin == "" {
		return fmt.Sprintf(`execute: %q with no args and no stdin would drop into a REPL and exit silently. To run a script, pass its path in args, e.g. {"cmd":%q,"args":["solution.py"]}. To run inline code, use {"cmd":%q,"args":["-c","print('hi')"]}.`, cmd, cmd, cmd)
	}
	return ""
}

// stripTrailingVersion drops a trailing ".N", ".N.M", or "-N" version
// segment from a basename. Examples: "python3.12" -> "python3";
// "ruby-3.3.0" -> "ruby"; "node-20" -> "node"; "python" -> "python".
//
// Both dot- and hyphen-separated digit suffixes are stripped so a
// model that emits {"cmd":"node-20"} hits the same silent-REPL lint
// as {"cmd":"node"}.
func stripTrailingVersion(s string) string {
	for {
		dot := strings.LastIndexByte(s, '.')
		if dot <= 0 || dot == len(s)-1 {
			break
		}
		tail := s[dot+1:]
		if !allDigits(tail) {
			break
		}
		s = s[:dot]
	}
	if hy := strings.LastIndexByte(s, '-'); hy > 0 && hy < len(s)-1 && allDigits(s[hy+1:]) {
		s = s[:hy]
	}
	return s
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
