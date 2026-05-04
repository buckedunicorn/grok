// memory demonstrates in-process and file-backed memory stores used as
// conversation context. The agent "remembers" facts across messages by
// injecting stored facts into the system prompt via AsSystemFragment.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	grok "github.com/buckedunicorn/grok"
	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/memory"
)

func main() {
	client := grok.New(grok.WithAPIKey(os.Getenv("XAI_API_KEY")))

	// --- InMemory store ---
	// InMemory.Set always returns nil; the error is part of the Store
	// interface so File and other persistent backends can surface I/O
	// failures.
	mem := &memory.InMemory{}
	_ = mem.Set("user_name", "Alice")
	_ = mem.Set("preferred_language", "Go")
	_ = mem.Set("timezone", "UTC+1")

	// Inject facts as a system prompt fragment.
	system := "You are a personalized assistant.\n\n" + memory.AsSystemFragment(mem)
	cv := chat.NewConversation(client.Chat, "grok-4-1-fast-reasoning", chat.WithSystem(system))

	comp, err := cv.Send(context.Background(), "What's my name and what language do I prefer?")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("InMemory response:", comp.Choices[0].Message.Content)

	// --- File-backed store (persists across process restarts) ---
	path := filepath.Join(os.TempDir(), "grok-memory-example.json")
	defer os.Remove(path)

	fmem, err := memory.NewFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "file store:", err)
		os.Exit(1)
	}

	if err := fmem.Set("project", "grok SDK"); err != nil {
		fmt.Fprintln(os.Stderr, "file store set:", err)
		os.Exit(1)
	}
	if err := fmem.Set("deadline", "end of Q2"); err != nil {
		fmt.Fprintln(os.Stderr, "file store set:", err)
		os.Exit(1)
	}

	// Simulate reopening (e.g. after a process restart).
	fmem2, _ := memory.NewFile(path)
	fmt.Println("\nFile store (reloaded):")
	for _, e := range fmem2.All() {
		fmt.Printf("  %s = %s\n", e.Key, e.Value)
	}

	// Demonstrate fragment injection.
	fmt.Println("\nSystem fragment:\n" + memory.AsSystemFragment(fmem2))
}
