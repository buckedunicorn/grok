// Package memory provides conversation and agent memory backends.
//
// Use InMemory for ephemeral in-process storage and File for persistence
// across process restarts. Either backend satisfies the Store interface and
// can be used with AsSystemFragment to inject facts into a chat system prompt.
//
// # Trust boundary
//
// AsSystemFragment renders every key and value verbatim into the
// system prompt that the model sees. Anyone able to call Store.Set
// can therefore inject system-level instructions for the next turn
// . Treat Set as a trusted-code-only entry point: do
// not expose it to untrusted users without sanitising the input or
// scoping the store per user.
package memory

import (
	"fmt"
	"strings"
)

// Store is a simple key-value memory for an agent or chatbot.
//
// Set, Delete, and Clear return error so persistent backends (File, future
// SQL/KV adapters) can surface I/O failures. Pure in-process backends like
// InMemory always return nil, callers writing only against InMemory can
// treat the error as advisory.
type Store interface {
	// Set stores a value under key, overwriting any prior value.
	Set(key, value string) error
	// Get returns the value for key, or "" if not found.
	Get(key string) string
	// All returns all stored entries in insertion order.
	All() []Entry
	// Delete removes key. No-op if not present.
	Delete(key string) error
	// Clear removes all entries.
	Clear() error
}

// Entry is a single key-value memory record.
type Entry struct {
	Key   string
	Value string
}

// AsSystemFragment formats all entries as a compact system-prompt section.
// Returns "" when the store is empty.
func AsSystemFragment(s Store) string {
	entries := s.All()
	if len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Remembered facts:\n")
	for _, e := range entries {
		fmt.Fprintf(&sb, "- %s: %s\n", e.Key, e.Value)
	}
	return sb.String()
}
