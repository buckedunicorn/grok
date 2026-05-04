package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// File is a thread-safe memory store backed by a JSON file.
//
// Changes are flushed to disk on every Set/Delete/Clear call via an
// atomic temp-file-plus-rename so a crash mid-write cannot corrupt
// the file. The parent directory is opened via *os.Root so symlink
// swaps inside the dir cannot redirect writes outside it.
type File struct {
	base string // basename of the JSON file inside the rooted dir
	root *os.Root
	mu   sync.RWMutex
	data map[string]string
	keys []string
}

// NewFile opens or creates the JSON store at path.
// Existing data is loaded immediately; an empty or missing file is valid.
//
// path's parent directory is opened via os.Root for the lifetime of
// the File; subsequent renames cannot follow a symlink swapped in for
// the dir.
func NewFile(path string) (*File, error) {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("memory/file: create dir: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("memory/file: open root: %w", err)
	}
	f := &File{
		base: filepath.Base(path),
		root: root,
		data: make(map[string]string),
	}
	if err := f.load(); err != nil {
		_ = root.Close()
		return nil, err
	}
	return f, nil
}

// Close releases the os.Root handle on the parent directory.
// Optional: short-lived processes can leak this on shutdown.
func (f *File) Close() error {
	if f == nil || f.root == nil {
		return nil
	}
	return f.root.Close()
}

// Set satisfies Store. Returns an error if the on-disk write fails, in
// that case the in-memory state is rolled back so the store stays
// consistent with what's durable.
func (f *File) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	prev, hadPrev := f.data[key]
	if !hadPrev {
		f.keys = append(f.keys, key)
	}
	f.data[key] = value

	if err := f.flush(); err != nil {
		// Roll back so memory matches disk.
		if hadPrev {
			f.data[key] = prev
		} else {
			delete(f.data, key)
			f.keys = f.keys[:len(f.keys)-1]
		}
		return err
	}
	return nil
}

// Get satisfies Store.
func (f *File) Get(key string) string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.data[key]
}

// All satisfies Store.
func (f *File) All() []Entry {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]Entry, 0, len(f.keys))
	for _, k := range f.keys {
		if v, ok := f.data[k]; ok {
			out = append(out, Entry{Key: k, Value: v})
		}
	}
	return out
}

// Delete satisfies Store. No-op (and no flush) if key is absent. On flush
// failure, in-memory state is rolled back to match disk.
func (f *File) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	prev, hadPrev := f.data[key]
	if !hadPrev {
		return nil
	}
	prevKeys := make([]string, len(f.keys))
	copy(prevKeys, f.keys)

	delete(f.data, key)
	for i, k := range f.keys {
		if k == key {
			f.keys = append(f.keys[:i], f.keys[i+1:]...)
			break
		}
	}

	if err := f.flush(); err != nil {
		f.data[key] = prev
		f.keys = prevKeys
		return err
	}
	return nil
}

// Clear satisfies Store. On flush failure, in-memory state is rolled back.
func (f *File) Clear() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	prevData := f.data
	prevKeys := f.keys
	f.data = make(map[string]string)
	f.keys = nil

	if err := f.flush(); err != nil {
		f.data = prevData
		f.keys = prevKeys
		return err
	}
	return nil
}

// on-disk format
type fileFormat struct {
	Keys []string          `json:"keys"`
	Data map[string]string `json:"data"`
}

func (f *File) load() error {
	file, err := f.root.Open(f.base)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("memory/file: opening %s: %w", f.base, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("memory/file: reading %s: %w", f.base, err)
	}
	if len(raw) == 0 {
		return nil
	}
	var ff fileFormat
	if err := json.Unmarshal(raw, &ff); err != nil {
		return fmt.Errorf("memory/file: parsing %s: %w", f.base, err)
	}
	f.data = ff.Data
	f.keys = ff.Keys
	if f.data == nil {
		f.data = make(map[string]string)
	}
	return nil
}

// flush writes current state to disk atomically via temp file + rename.
// The parent dir was captured as *os.Root at construction so the
// rename target cannot be redirected via a symlink swap.
// Caller must hold mu.Lock.
func (f *File) flush() error {
	body, err := json.MarshalIndent(fileFormat{Keys: f.keys, Data: f.data}, "", "  ")
	if err != nil {
		return fmt.Errorf("memory/file: marshal: %w", err)
	}

	tmpBase := "." + f.base + ".tmp"
	tmp, err := f.root.OpenFile(tmpBase, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("memory/file: create temp: %w", err)
	}
	cleanup := func() {
		if tmpBase != "" {
			_ = f.root.Remove(tmpBase)
		}
	}
	defer cleanup()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("memory/file: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("memory/file: fsync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("memory/file: close temp: %w", err)
	}
	if err := f.root.Rename(tmpBase, f.base); err != nil {
		return fmt.Errorf("memory/file: rename: %w", err)
	}
	tmpBase = "" // disarm cleanup
	return nil
}
