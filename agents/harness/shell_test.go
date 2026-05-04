package harness_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/buckedunicorn/grok/agents/harness"
)

func TestLocalExecutor_basicEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo behavior differs on Windows")
	}
	e := &harness.LocalExecutor{}
	stdout, stderr, err := e.Run(context.Background(), "echo", []string{"hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdout), "hello") {
		t.Errorf("stdout = %q", stdout)
	}
	if len(stderr) != 0 {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestLocalExecutor_stdinPiped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cat not standard on Windows")
	}
	e := &harness.LocalExecutor{}
	stdout, _, err := e.Run(context.Background(), "cat", nil, []byte("piped\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdout), "piped") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestLocalExecutor_ctxCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep behavior differs on Windows")
	}
	e := &harness.LocalExecutor{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err := e.Run(ctx, "sleep", []string{"5"}, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestLocalExecutor_workdir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pwd is not standard on Windows")
	}
	tmp := t.TempDir()
	e := &harness.LocalExecutor{Workdir: tmp}
	stdout, _, err := e.Run(context.Background(), "pwd", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// macOS resolves /var/folders -> /private/var/folders, so use Contains.
	got := strings.TrimSpace(string(stdout))
	if !strings.HasSuffix(got, strings.TrimPrefix(tmp, "/private")) {
		t.Errorf("pwd = %q, expected suffix from %q", got, tmp)
	}
}

func TestLocalExecutor_nonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false not standard on Windows")
	}
	e := &harness.LocalExecutor{}
	_, _, err := e.Run(context.Background(), "false", nil, nil)
	if err == nil {
		t.Error("expected non-nil error for non-zero exit")
	}
}
