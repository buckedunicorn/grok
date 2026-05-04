package cache_test

import (
	"testing"

	"github.com/buckedunicorn/grok/cache"
	"github.com/buckedunicorn/grok/chat"
)

func TestManager_convID_unique(t *testing.T) {
	m1 := cache.NewManager()
	m2 := cache.NewManager()
	if m1.ConvID() == m2.ConvID() {
		t.Error("two managers should have distinct conv IDs")
	}
	if m1.ConvID() == "" {
		t.Error("ConvID must not be empty")
	}
}

func TestManager_record_and_stats(t *testing.T) {
	m := cache.NewManager()

	m.Record(chat.Usage{
		PromptTokens:        100,
		PromptTokensDetails: chat.PromptTokensDetails{CachedTokens: 80},
	})
	m.Record(chat.Usage{
		PromptTokens:        200,
		PromptTokensDetails: chat.PromptTokensDetails{CachedTokens: 100},
	})

	s := m.Stats()
	if s.Calls != 2 {
		t.Errorf("Calls=%d want 2", s.Calls)
	}
	if s.TotalPromptTokens != 300 {
		t.Errorf("TotalPromptTokens=%d want 300", s.TotalPromptTokens)
	}
	if s.CachedTokens != 180 {
		t.Errorf("CachedTokens=%d want 180", s.CachedTokens)
	}
	rate := s.HitRate()
	if rate < 0.59 || rate > 0.61 {
		t.Errorf("HitRate=%.4f want ~0.60", rate)
	}
}

func TestManager_hitRate_zero_when_no_tokens(t *testing.T) {
	m := cache.NewManager()
	if m.Stats().HitRate() != 0 {
		t.Error("HitRate should be 0 when no tokens recorded")
	}
}

func TestManager_reset(t *testing.T) {
	m := cache.NewManager()
	m.Record(chat.Usage{PromptTokens: 100})
	m.Reset()
	s := m.Stats()
	if s.Calls != 0 || s.TotalPromptTokens != 0 {
		t.Errorf("after Reset: %+v", s)
	}
}

func TestStats_string(t *testing.T) {
	s := cache.Stats{Calls: 1, TotalPromptTokens: 100, CachedTokens: 50}
	str := s.String()
	if str == "" {
		t.Error("Stats.String() should not be empty")
	}
}
