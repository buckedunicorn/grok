package memory

import "sync"

// InMemory is a thread-safe in-process memory store.
// The zero value is ready to use.
type InMemory struct {
	mu   sync.RWMutex
	data map[string]string
	keys []string // preserves insertion order
}

// Set satisfies Store. Always returns nil.
func (m *InMemory) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]string)
	}
	if _, exists := m.data[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.data[key] = value
	return nil
}

// Get satisfies Store.
func (m *InMemory) Get(key string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data[key]
}

// All satisfies Store.
func (m *InMemory) All() []Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Entry, 0, len(m.keys))
	for _, k := range m.keys {
		if v, ok := m.data[k]; ok {
			out = append(out, Entry{Key: k, Value: v})
		}
	}
	return out
}

// Delete satisfies Store. Always returns nil.
func (m *InMemory) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
	return nil
}

// Clear satisfies Store. Always returns nil.
func (m *InMemory) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = nil
	m.keys = nil
	return nil
}
