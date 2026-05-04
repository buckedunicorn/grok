package memory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buckedunicorn/grok/memory"
)

func testStore(t *testing.T, s memory.Store) {
	t.Helper()

	s.Set("name", "Alice")
	s.Set("city", "Paris")

	if got := s.Get("name"); got != "Alice" {
		t.Errorf("Get(name)=%q want Alice", got)
	}
	if got := s.Get("city"); got != "Paris" {
		t.Errorf("Get(city)=%q want Paris", got)
	}
	if got := s.Get("missing"); got != "" {
		t.Errorf("Get(missing)=%q want empty", got)
	}

	all := s.All()
	if len(all) != 2 {
		t.Fatalf("All() len=%d want 2", len(all))
	}
	if all[0].Key != "name" || all[1].Key != "city" {
		t.Errorf("unexpected order: %v", all)
	}

	s.Set("name", "Bob") // overwrite
	if got := s.Get("name"); got != "Bob" {
		t.Errorf("overwrite: Get(name)=%q want Bob", got)
	}
	if len(s.All()) != 2 {
		t.Errorf("overwrite should not add a new key, got %d keys", len(s.All()))
	}

	s.Delete("city")
	if got := s.Get("city"); got != "" {
		t.Errorf("after delete: Get(city)=%q want empty", got)
	}
	if len(s.All()) != 1 {
		t.Errorf("after delete: want 1 key, got %d", len(s.All()))
	}

	s.Clear()
	if len(s.All()) != 0 {
		t.Errorf("after Clear: want 0 keys, got %d", len(s.All()))
	}
}

func TestInMemory(t *testing.T) {
	testStore(t, &memory.InMemory{})
}

func TestFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mem.json")
	s, err := memory.NewFile(path)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	testStore(t, s)
}

func TestFile_persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mem.json")

	s1, _ := memory.NewFile(path)
	s1.Set("key", "value")

	s2, err := memory.NewFile(path)
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	if got := s2.Get("key"); got != "value" {
		t.Errorf("persistence: Get(key)=%q want value", got)
	}
}

func TestFile_emptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mem.json")
	os.WriteFile(path, []byte{}, 0o644)

	_, err := memory.NewFile(path)
	if err != nil {
		t.Fatalf("empty file should be valid: %v", err)
	}
}

func TestAsSystemFragment_empty(t *testing.T) {
	s := &memory.InMemory{}
	if got := memory.AsSystemFragment(s); got != "" {
		t.Errorf("empty store should produce empty fragment, got %q", got)
	}
}

func TestAsSystemFragment_populated(t *testing.T) {
	s := &memory.InMemory{}
	s.Set("user", "Alice")
	s.Set("lang", "Go")

	frag := memory.AsSystemFragment(s)
	if !strings.Contains(frag, "Alice") || !strings.Contains(frag, "Go") {
		t.Errorf("fragment missing entries: %q", frag)
	}
}
