package harness

import "sync"

// State holds per-run data shared between tools. One State is created per
// Harness.Run call; nothing leaks between runs.
//
// State is safe for concurrent use, all mutations are mutex-guarded so a
// model that issues parallel tool calls within a single turn cannot corrupt
// the shared list/map.
type State struct {
	mu    sync.Mutex
	todos []Todo
}

// Todo is a single planning entry.
type Todo struct {
	Index   int    `json:"index"`
	Content string `json:"content"`
	Status  string `json:"status"` // "pending" | "in_progress" | "done"
}

func newState() *State {
	return &State{}
}

// Todos returns a snapshot of the todo list.
func (s *State) Todos() []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Todo, len(s.todos))
	copy(out, s.todos)
	return out
}

// SetTodos replaces the todo list. Used by write_todos.
func (s *State) SetTodos(items []Todo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range items {
		items[i].Index = i
		if items[i].Status == "" {
			items[i].Status = "pending"
		}
	}
	s.todos = items
}
