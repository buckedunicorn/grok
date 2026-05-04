package harness

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/buckedunicorn/grok/agents"
)

// FS is the filesystem surface available to the harness's filesystem tools.
//
// Two implementations ship: MemoryFS (default, in-memory, scoped per-run)
// and LocalFS (opt-in, rooted at a single directory). Custom backends
// (S3, sandbox-in-a-VM, etc.) just need to implement these six methods.
type FS interface {
	ReadFile(name string) (string, error)
	WriteFile(name string, content string) error
	EditFile(name string, oldStr, newStr string) error
	List(dir string) ([]string, error)
	Glob(pattern string) ([]string, error)
	Grep(pattern, dir string) ([]GrepMatch, error)
}

// GrepMatch is a single line match from FS.Grep.
type GrepMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// ErrNotFound is returned by FS implementations when a path does not exist.
var ErrNotFound = errors.New("harness/fs: not found")

// ---- MemoryFS --------------------------------------------------------------

// MemoryFS is an in-memory filesystem keyed by absolute-style path strings
// ("foo.txt", "src/main.go"). Safe for concurrent use.
type MemoryFS struct {
	mu    sync.Mutex
	files map[string]string
}

// NewMemoryFS returns an empty MemoryFS.
func NewMemoryFS() *MemoryFS {
	return &MemoryFS{files: map[string]string{}}
}

func (m *MemoryFS) ReadFile(name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.files[normalizePath(name)]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return v, nil
}

func (m *MemoryFS) WriteFile(name, content string) error {
	m.mu.Lock()
	m.files[normalizePath(name)] = content
	m.mu.Unlock()
	return nil
}

func (m *MemoryFS) EditFile(name, oldStr, newStr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := normalizePath(name)
	cur, ok := m.files[key]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if !strings.Contains(cur, oldStr) {
		return fmt.Errorf("harness/fs: edit_file: oldStr not found in %s", name)
	}
	m.files[key] = strings.Replace(cur, oldStr, newStr, 1)
	return nil
}

func (m *MemoryFS) List(dir string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dir = normalizePath(dir)
	if dir == "." {
		dir = ""
	}
	prefix := dir
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	seen := map[string]struct{}{}
	for k := range m.files {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		// First path segment after prefix.
		if i := strings.Index(rest, "/"); i >= 0 {
			rest = rest[:i] + "/"
		}
		seen[rest] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

func (m *MemoryFS) Glob(pattern string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []string{}
	for k := range m.files {
		ok, err := path.Match(pattern, k)
		if err != nil {
			return nil, fmt.Errorf("harness/fs: glob: %w", err)
		}
		if ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (m *MemoryFS) Grep(pattern, dir string) ([]GrepMatch, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("harness/fs: grep: %w", err)
	}
	dir = normalizePath(dir)
	prefix := dir
	if prefix == "." {
		prefix = ""
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	// Snapshot the matching files under the lock, then run the regex
	// outside it. Holding the lock during regex
	// matching across many files would block all other FS ops.
	type kv struct {
		path    string
		content string
	}
	m.mu.Lock()
	snapshot := make([]kv, 0, len(m.files))
	for k, v := range m.files {
		if strings.HasPrefix(k, prefix) {
			snapshot = append(snapshot, kv{path: k, content: v})
		}
	}
	m.mu.Unlock()

	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].path < snapshot[j].path })

	out := []GrepMatch{}
	for _, e := range snapshot {
		// strings.SplitSeq iterates lazily; the per-line string still
		// allocates (substrings of an immutable string), but we avoid
		// the slice-of-strings header that strings.Split builds.
		i := 0
		for line := range strings.SplitSeq(e.content, "\n") {
			i++
			if re.MatchString(line) {
				out = append(out, GrepMatch{Path: e.path, Line: i, Text: line})
			}
		}
	}
	return out, nil
}

// ---- LocalFS ---------------------------------------------------------------

// LocalFS is a rooted filesystem implementation backed by os.Root. All
// operations are confined to Root; paths that escape Root via traversal
// or via symlinks pointing outside Root are rejected by the kernel
// (Go 1.24+ os.Root semantics).
//
// The default file mode for newly created files is 0o600 and for
// directories 0o700; widen via WithLocalFSFileMode and
// WithLocalFSDirMode if your workload needs world-readable artifacts.
//
// WriteFile rejects content larger than MaxFileBytes (default 32 MiB).
// Grep skips files larger than MaxGrepFileBytes (default 4 MiB) and
// files whose first 4 KiB contain a NUL byte (binary heuristic).
type LocalFS struct {
	// Root is the canonical absolute path the filesystem is rooted at.
	// Set by NewLocalFS. Read-only after construction.
	Root string

	root *os.Root

	fileMode    os.FileMode
	dirMode     os.FileMode
	maxFileSize int64
	maxGrepSize int64
}

// LocalFSOption configures a LocalFS.
type LocalFSOption func(*LocalFS)

// WithLocalFSFileMode sets the mode for files created by WriteFile.
// Defaults to 0o600 (owner-only).
func WithLocalFSFileMode(mode os.FileMode) LocalFSOption {
	return func(l *LocalFS) { l.fileMode = mode }
}

// WithLocalFSDirMode sets the mode for directories created by
// WriteFile (when ancestor dirs do not exist). Defaults to 0o700
// (owner-only).
func WithLocalFSDirMode(mode os.FileMode) LocalFSOption {
	return func(l *LocalFS) { l.dirMode = mode }
}

// WithLocalFSMaxFileBytes caps the size of a single WriteFile call.
// Writes that exceed the cap are rejected with an error before any
// disk I/O. Defaults to 32 MiB. Pass <= 0 to disable.
func WithLocalFSMaxFileBytes(n int64) LocalFSOption {
	return func(l *LocalFS) { l.maxFileSize = n }
}

// WithLocalFSMaxGrepFileBytes caps the size of files inspected by
// Grep. Larger files are silently skipped. Defaults to 4 MiB.
func WithLocalFSMaxGrepFileBytes(n int64) LocalFSOption {
	return func(l *LocalFS) { l.maxGrepSize = n }
}

// NewLocalFS returns a LocalFS rooted at root. root must be an
// absolute path to an existing directory. Subsequent operations are
// confined inside root by the kernel via os.Root.
func NewLocalFS(root string, opts ...LocalFSOption) (*LocalFS, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("harness/fs: LocalFS root must be absolute: %q", root)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("harness/fs: LocalFS stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("harness/fs: LocalFS root is not a directory: %q", root)
	}
	abs, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("harness/fs: LocalFS resolve root: %w", err)
	}
	r, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("harness/fs: LocalFS open root: %w", err)
	}
	l := &LocalFS{
		Root:        abs,
		root:        r,
		fileMode:    0o600,
		dirMode:     0o700,
		maxFileSize: 32 * 1024 * 1024,
		maxGrepSize: 4 * 1024 * 1024,
	}
	for _, o := range opts {
		o(l)
	}
	return l, nil
}

// Close releases the OS handle backing the rooted filesystem. Optional
// for tests; long-lived agents typically leak this on shutdown.
func (l *LocalFS) Close() error {
	if l == nil || l.root == nil {
		return nil
	}
	return l.root.Close()
}

// cleanRel turns a caller-supplied path into a value safe to pass to
// os.Root. The kernel itself enforces containment, but we strip any
// leading separator so absolute-looking paths are interpreted as
// relative to root.
func cleanRel(name string) string {
	clean := filepath.Clean("/" + name)
	return strings.TrimPrefix(clean, string(os.PathSeparator))
}

func (l *LocalFS) ReadFile(name string) (string, error) {
	f, err := l.root.Open(cleanRel(name))
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return "", fmt.Errorf("harness/fs: read_file: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (l *LocalFS) WriteFile(name, content string) error {
	if l.maxFileSize > 0 && int64(len(content)) > l.maxFileSize {
		return fmt.Errorf("harness/fs: write_file %q: content exceeds %d bytes", name, l.maxFileSize)
	}
	rel := cleanRel(name)
	if dir := filepath.Dir(rel); dir != "." && dir != "" {
		if err := l.mkdirAll(dir); err != nil {
			return err
		}
	}
	f, err := l.root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, l.fileMode)
	if err != nil {
		return fmt.Errorf("harness/fs: write_file: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return err
	}
	return nil
}

// mkdirAll mkdir-ps each ancestor of dir under the root.
func (l *LocalFS) mkdirAll(dir string) error {
	parts := strings.Split(dir, string(os.PathSeparator))
	cur := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if cur == "" {
			cur = p
		} else {
			cur = cur + string(os.PathSeparator) + p
		}
		if err := l.root.Mkdir(cur, l.dirMode); err != nil && !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("harness/fs: mkdir %q: %w", cur, err)
		}
	}
	return nil
}

func (l *LocalFS) EditFile(name, oldStr, newStr string) error {
	cur, err := l.ReadFile(name)
	if err != nil {
		return err
	}
	if !strings.Contains(cur, oldStr) {
		return fmt.Errorf("harness/fs: edit_file: oldStr not found in %s", name)
	}
	return l.WriteFile(name, strings.Replace(cur, oldStr, newStr, 1))
}

func (l *LocalFS) List(dir string) ([]string, error) {
	rel := cleanRel(dir)
	if rel == "" {
		rel = "."
	}
	f, err := l.root.Open(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, dir)
	}
	if err != nil {
		return nil, fmt.Errorf("harness/fs: ls: %w", err)
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func (l *LocalFS) Glob(pattern string) ([]string, error) {
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, fmt.Errorf("harness/fs: glob: %w", err)
	}
	var out []string
	rfs := l.root.FS()
	err := fs.WalkDir(rfs, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if ok, _ := path.Match(pattern, p); ok {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("harness/fs: glob: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

func (l *LocalFS) Grep(pattern, dir string) ([]GrepMatch, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("harness/fs: grep: %w", err)
	}
	rel := cleanRel(dir)
	if rel == "" {
		rel = "."
	}
	out := []GrepMatch{}
	rfs := l.root.FS()
	walkErr := fs.WalkDir(rfs, rel, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if l.maxGrepSize > 0 && info.Size() > l.maxGrepSize {
			return nil
		}
		f, err := l.root.Open(p)
		if err != nil {
			return nil
		}
		matches, lerr := grepFile(f, re, p)
		_ = f.Close()
		if lerr != nil {
			return nil
		}
		out = append(out, matches...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return out, nil
}

// grepFile streams f line by line and collects regex matches. It
// performs a binary-content sniff on the first 4 KiB and skips the
// file when a NUL byte appears.
func grepFile(f io.Reader, re *regexp.Regexp, name string) ([]GrepMatch, error) {
	br := bufio.NewReader(f)
	head, _ := br.Peek(4096)
	if bytes.IndexByte(head, 0) >= 0 {
		return nil, nil
	}
	scanner := bufio.NewScanner(br)
	const maxLine = 1 << 20 // 1 MiB per line
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)
	var out []GrepMatch
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		if re.MatchString(text) {
			out = append(out, GrepMatch{Path: name, Line: line, Text: text})
		}
	}
	return out, nil
}

// ---- helpers ---------------------------------------------------------------

func normalizePath(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(p)), "./")
}

// fixEscapeBlunder undoes a common LLM tool-call mistake: emitting JSON args
// where what should be a newline escape ("\n") is double-escaped ("\\n"),
// causing json.Unmarshal to produce a Go string with literal backslash-n
// instead of an actual newline byte. The same applies to "\t" and "\r".
//
// The fix is conservative: only normalize when the string contains no real
// newlines AND has at least one literal "\n" sequence. If the model wrote
// genuine multi-line content (real newlines present) we trust its escaping
// and don't touch the string, that preserves legitimate cases like source
// code or regex patterns that intentionally include the literal "\n"
// substring.
//
// Applied to write_file content and edit_file old_str/new_str so the tools
// are robust to the most common escape mistake without surprising users
// who genuinely meant the literal sequence.
func fixEscapeBlunder(s string) string {
	if strings.ContainsRune(s, '\n') || strings.ContainsRune(s, '\t') {
		return s
	}
	if !strings.Contains(s, `\n`) && !strings.Contains(s, `\t`) && !strings.Contains(s, `\r`) {
		return s
	}
	r := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r")
	return r.Replace(s)
}

// ---- tools -----------------------------------------------------------------

func fsTools(f FS) []agents.Tool {
	return []agents.Tool{
		readFileTool(f),
		writeFileTool(f),
		editFileTool(f),
		lsTool(f),
		globTool(f),
		grepTool(f),
	}
}

func readFileTool(f FS) agents.Tool {
	return agents.Tool{
		Name:        "read_file",
		Description: "Read the contents of a file as a string.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
			"required": []string{"path"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", err
			}
			content, err := f.ReadFile(p.Path)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(map[string]any{"path": p.Path, "content": content})
			return string(out), nil
		},
	}
}

func writeFileTool(f FS) agents.Tool {
	return agents.Tool{
		Name:        "write_file",
		Description: "Write content to a file, overwriting if it exists. Creates parent directories.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
				"content": map[string]any{
					"type": "string",
					"description": "Exact bytes to write. To include a newline in the file, " +
						"use the standard JSON escape \"\\n\" (which decodes to a real newline). " +
						"Do NOT emit \"\\\\n\", that writes a literal backslash followed by 'n', " +
						"not a newline. Same rule for tabs (\"\\t\"), quotes (\"\\\"\"), etc.",
				},
			},
			"required": []string{"path", "content"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", err
			}
			content := fixEscapeBlunder(p.Content)
			if err := f.WriteFile(p.Path, content); err != nil {
				return "", err
			}
			out, _ := json.Marshal(map[string]any{
				"ok":    true,
				"path":  p.Path,
				"bytes": len(content),
			})
			return string(out), nil
		},
	}
}

func editFileTool(f FS) agents.Tool {
	return agents.Tool{
		Name: "edit_file",
		Description: "Replace the first occurrence of old_str with new_str in path. " +
			"old_str must match exactly; use enough context to be unique.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"old_str": map[string]any{"type": "string"},
				"new_str": map[string]any{"type": "string"},
			},
			"required": []string{"path", "old_str", "new_str"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct {
				Path   string `json:"path"`
				OldStr string `json:"old_str"`
				NewStr string `json:"new_str"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", err
			}
			oldStr := fixEscapeBlunder(p.OldStr)
			newStr := fixEscapeBlunder(p.NewStr)
			if err := f.EditFile(p.Path, oldStr, newStr); err != nil {
				return "", err
			}
			out, _ := json.Marshal(map[string]any{"ok": true, "path": p.Path})
			return string(out), nil
		},
	}
}

func lsTool(f FS) agents.Tool {
	return agents.Tool{
		Name:        "ls",
		Description: "List entries in a directory. Subdirectories are returned with a trailing slash.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dir": map[string]any{"type": "string", "description": "Directory path. Empty or '.' means root."},
			},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct {
				Dir string `json:"dir"`
			}
			_ = json.Unmarshal([]byte(args), &p)
			entries, err := f.List(p.Dir)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(map[string]any{"dir": p.Dir, "entries": entries})
			return string(out), nil
		},
	}
}

func globTool(f FS) agents.Tool {
	return agents.Tool{
		Name:        "glob",
		Description: "Return paths matching a shell-style glob pattern (e.g. 'src/*.go').",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
			},
			"required": []string{"pattern"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct {
				Pattern string `json:"pattern"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", err
			}
			matches, err := f.Glob(p.Pattern)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(map[string]any{"pattern": p.Pattern, "matches": matches})
			return string(out), nil
		},
	}
}

func grepTool(f FS) agents.Tool {
	return agents.Tool{
		Name:        "grep",
		Description: "Search files under dir for lines matching the regex pattern.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Go regexp pattern."},
				"dir":     map[string]any{"type": "string", "description": "Directory to search. Empty means root."},
			},
			"required": []string{"pattern"},
		},
		Handler: func(_ context.Context, args string) (string, error) {
			var p struct {
				Pattern string `json:"pattern"`
				Dir     string `json:"dir"`
			}
			if err := json.Unmarshal([]byte(args), &p); err != nil {
				return "", err
			}
			matches, err := f.Grep(p.Pattern, p.Dir)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(map[string]any{"pattern": p.Pattern, "matches": matches})
			return string(out), nil
		},
	}
}
