package harness_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/agents/harness"
)

func TestMemoryFS_writeReadEdit(t *testing.T) {
	fs := harness.NewMemoryFS()

	if err := fs.WriteFile("notes.md", "hello"); err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile("notes.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf("got %q", got)
	}

	if err := fs.EditFile("notes.md", "hello", "hi"); err != nil {
		t.Fatal(err)
	}
	got, _ = fs.ReadFile("notes.md")
	if got != "hi" {
		t.Errorf("after edit, got %q", got)
	}
}

func TestMemoryFS_readMissing(t *testing.T) {
	fs := harness.NewMemoryFS()
	_, err := fs.ReadFile("nope.txt")
	if !errors.Is(err, harness.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestMemoryFS_editOldStrNotFound(t *testing.T) {
	fs := harness.NewMemoryFS()
	_ = fs.WriteFile("a.txt", "abc")
	err := fs.EditFile("a.txt", "xyz", "QQQ")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v", err)
	}
}

func TestMemoryFS_listAndGlob(t *testing.T) {
	fs := harness.NewMemoryFS()
	for _, p := range []string{"src/main.go", "src/util.go", "README.md"} {
		_ = fs.WriteFile(p, "x")
	}

	root, _ := fs.List("")
	wantRoot := map[string]bool{"src/": true, "README.md": true}
	if len(root) != 2 {
		t.Errorf("root entries = %v", root)
	}
	for _, e := range root {
		if !wantRoot[e] {
			t.Errorf("unexpected entry %q", e)
		}
	}

	src, _ := fs.List("src")
	if len(src) != 2 {
		t.Errorf("src entries = %v", src)
	}

	matches, err := fs.Glob("src/*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 {
		t.Errorf("glob = %v", matches)
	}
}

func TestMemoryFS_grep(t *testing.T) {
	fs := harness.NewMemoryFS()
	_ = fs.WriteFile("a.go", "package main\nfunc main() {}")
	_ = fs.WriteFile("b.go", "package main\nvar x = 1")

	got, err := fs.Grep(`^func`, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "a.go" || got[0].Line != 2 {
		t.Errorf("grep = %+v", got)
	}
}

func TestLocalFS_resolveRefusesEscape(t *testing.T) {
	root := t.TempDir()
	fs, err := harness.NewLocalFS(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fs.ReadFile("../../etc/passwd")
	if err == nil {
		t.Fatal("expected escape rejection")
	}
}

func TestLocalFS_writeReadList(t *testing.T) {
	root := t.TempDir()
	fs, err := harness.NewLocalFS(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := fs.WriteFile("sub/hello.txt", "world"); err != nil {
		t.Fatal(err)
	}
	got, err := fs.ReadFile("sub/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "world" {
		t.Errorf("got %q", got)
	}

	// verify on disk
	disk, err := os.ReadFile(filepath.Join(root, "sub/hello.txt"))
	if err != nil || string(disk) != "world" {
		t.Errorf("disk = %q err = %v", disk, err)
	}

	entries, _ := fs.List("sub")
	if len(entries) != 1 || entries[0] != "hello.txt" {
		t.Errorf("entries = %v", entries)
	}
}

func TestLocalFS_grep(t *testing.T) {
	root := t.TempDir()
	fs, _ := harness.NewLocalFS(root)
	_ = fs.WriteFile("a.txt", "hello\nworld\n")
	_ = fs.WriteFile("b.txt", "nothing here")

	got, err := fs.Grep("hello|world", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("grep = %+v", got)
	}
}

func TestLocalFS_relativeRootRejected(t *testing.T) {
	_, err := harness.NewLocalFS("relative/path")
	if err == nil {
		t.Error("expected error for relative root")
	}
}

// regression: a symlink inside Root that points outside
// Root must not be followed by ReadFile.
func TestLocalFS_symlinkOutsideRootRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require admin on windows")
	}
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	fs, err := harness.NewLocalFS(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile("escape"); err == nil {
		t.Fatal("expected symlink-following to be rejected")
	}
}

// regression: Glob must not return paths outside Root.
func TestLocalFS_globRejectsEscape(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "inside.txt"), []byte("x"), 0o600)
	fs, _ := harness.NewLocalFS(root)
	matches, err := fs.Glob("../../*")
	if err == nil && len(matches) > 0 {
		t.Errorf("Glob should not enumerate outside Root, got %v", matches)
	}
	matches, err = fs.Glob("*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != "inside.txt" {
		t.Errorf("Glob = %v, want [inside.txt]", matches)
	}
}

// regression: WriteFile rejects content larger than the cap.
func TestLocalFS_writeFileSizeCapped(t *testing.T) {
	root := t.TempDir()
	fs, _ := harness.NewLocalFS(root, harness.WithLocalFSMaxFileBytes(16))
	if err := fs.WriteFile("ok.txt", "small"); err != nil {
		t.Fatalf("small write should succeed: %v", err)
	}
	if err := fs.WriteFile("toobig.txt", strings.Repeat("x", 32)); err == nil {
		t.Fatal("expected size-cap rejection")
	}
}

// regression: WriteFile defaults to owner-only mode.
func TestLocalFS_writeFileDefaultMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are POSIX-specific")
	}
	root := t.TempDir()
	fs, _ := harness.NewLocalFS(root)
	if err := fs.WriteFile("file.txt", "x"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("default file mode = %o, want 0o600", mode)
	}
}

// regression: Grep skips files larger than the cap and
// silently ignores binary files.
func TestLocalFS_grepBoundedAndSkipsBinary(t *testing.T) {
	root := t.TempDir()
	fs, _ := harness.NewLocalFS(root,
		harness.WithLocalFSMaxGrepFileBytes(64))
	_ = fs.WriteFile("text.txt", "hello\nworld\n")
	_ = os.WriteFile(filepath.Join(root, "binary.bin"),
		[]byte{0x00, 0x01, 0x02, 'h', 'e', 'l', 'l', 'o'}, 0o600)
	_ = os.WriteFile(filepath.Join(root, "huge.txt"),
		[]byte(strings.Repeat("hello\n", 100)), 0o600) // > 64 bytes

	got, err := fs.Grep("hello", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range got {
		if strings.Contains(m.Path, "binary") {
			t.Errorf("Grep returned binary match: %+v", m)
		}
		if strings.Contains(m.Path, "huge") {
			t.Errorf("Grep returned over-cap match: %+v", m)
		}
	}
}
