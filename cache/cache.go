// Package cache provides prompt-cache management for xAI Grok models.
//
// xAI automatically caches prompt prefixes (KV cache / TurboQuant-style
// quantized KV storage). Cache hits are most likely when the same
// x-grok-conv-id header is sent across turns in the same conversation.
//
// Manager tracks cache statistics across multiple completions and exposes
// hit-rate telemetry so you can verify caching is working as expected.
//
//	mgr := cache.NewManager()
//	client := grok.New(grok.WithConvID(mgr.ConvID()))
//	comp, _ := client.Chat.Create(ctx, req)
//	mgr.Record(comp.Usage)
//	fmt.Printf("cache hit rate: %.1f%%\n", mgr.Stats().HitRate()*100)
package cache

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/buckedunicorn/grok/chat"
)

// Manager tracks prompt-cache statistics for a single conversation.
type Manager struct {
	convID string
	mu     sync.Mutex

	totalPrompt int64
	cached      int64
	calls       atomic.Int64
}

// Stats holds aggregate cache metrics.
type Stats struct {
	Calls             int64
	TotalPromptTokens int64
	CachedTokens      int64
}

// HitRate returns the fraction of prompt tokens served from cache (0–1).
func (s Stats) HitRate() float64 {
	if s.TotalPromptTokens == 0 {
		return 0
	}
	return float64(s.CachedTokens) / float64(s.TotalPromptTokens)
}

func (s Stats) String() string {
	return fmt.Sprintf("calls=%d prompt_tokens=%d cached=%d hit_rate=%.1f%%",
		s.Calls, s.TotalPromptTokens, s.CachedTokens, s.HitRate()*100)
}

// NewManager creates a Manager with a fresh conversation ID.
// Pass the same ID to grok.WithConvID so all requests share a cache prefix.
func NewManager() *Manager {
	return &Manager{convID: newConvID()}
}

// ConvID returns the conversation ID to pass to grok.WithConvID.
func (m *Manager) ConvID() string { return m.convID }

// Record accumulates cache statistics from a chat completion usage block.
func (m *Manager) Record(u chat.Usage) {
	m.mu.Lock()
	m.totalPrompt += int64(u.PromptTokens)
	m.cached += int64(u.PromptTokensDetails.CachedTokens)
	m.mu.Unlock()
	m.calls.Add(1)
}

// Stats returns a snapshot of cumulative cache metrics.
func (m *Manager) Stats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Stats{
		Calls:             m.calls.Load(),
		TotalPromptTokens: m.totalPrompt,
		CachedTokens:      m.cached,
	}
}

// Reset clears all accumulated statistics (does not change ConvID).
func (m *Manager) Reset() {
	m.mu.Lock()
	m.totalPrompt = 0
	m.cached = 0
	m.mu.Unlock()
	m.calls.Store(0)
}
