package prompt_test

import (
	"testing"

	"github.com/buckedunicorn/grok/prompt"
)

// BenchmarkTemplate_Render validates single-pass
// substitution: the regex is matched once via FindAllStringSubmatchIndex
// and the substitution writes through a strings.Builder.
func BenchmarkTemplate_Render(b *testing.B) {
	t, _ := prompt.New("Translate {phrase} from {src} to {dst}, in the style of {style}.")
	vars := map[string]any{
		"phrase": "good morning",
		"src":    "english",
		"dst":    "japanese",
		"style":  "polite",
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = t.Render(vars)
	}
}

// BenchmarkChatTemplate_Render validates cached parse:
// repeated Render calls reuse the same parsed Template inside the
// ChatTemplate. Pre-fix every call re-parsed both System and User.
func BenchmarkChatTemplate_Render(b *testing.B) {
	ct, err := prompt.NewChatTemplate(
		"You are a {persona} assistant for a {domain} workflow.",
		"User asked about {topic}; respond in {tone}.",
	)
	if err != nil {
		b.Fatal(err)
	}
	vars := map[string]any{
		"persona": "concise",
		"domain":  "support",
		"topic":   "billing",
		"tone":    "friendly",
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ct.Render(vars)
	}
}

// BenchmarkChatTemplate_RenderZeroValue exercises the code path used
// by callers who construct &prompt.ChatTemplate{System: ..., User: ...}
// directly. Lazy parse-on-first-Render must still amortize across
// calls.
func BenchmarkChatTemplate_RenderZeroValue(b *testing.B) {
	ct := &prompt.ChatTemplate{
		System: "You are a {persona} assistant for a {domain} workflow.",
		User:   "User asked about {topic}; respond in {tone}.",
	}
	vars := map[string]any{
		"persona": "concise",
		"domain":  "support",
		"topic":   "billing",
		"tone":    "friendly",
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = ct.Render(vars)
	}
}
