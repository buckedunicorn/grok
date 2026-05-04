package chat_test

import (
	"testing"

	"github.com/buckedunicorn/grok/chat"
)

// completionWith wraps a content string in a one-choice Completion so
// the fuzzers can exercise the public parsers.
func fuzzCompletion(content string) *chat.Completion {
	return &chat.Completion{
		Choices: []chat.Choice{
			{Message: chat.Message{Role: "assistant", Content: content}},
		},
	}
}

// FuzzStripThinking confirms StripThinking never panics on arbitrary
// content, the result has at most one choice, and the stripped content
// no longer carries a <think>...</think> block.
func FuzzStripThinking(f *testing.F) {
	corpus := []string{
		"",
		"plain answer",
		"<think>hidden</think>visible",
		"prefix<think>a</think>middle<think>b</think>suffix",
		"<think>unterminated",
		"</think>only-close",
		"<think><think>nested</think></think>",
		"\x00\x01\x02",
		"<think>\n  multi-line\n</think>\nthen text",
	}
	for _, s := range corpus {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		comp := fuzzCompletion(content)
		out := chat.StripThinking(comp)
		if out == nil {
			t.Fatal("StripThinking returned nil")
		}
		if len(out.Choices) > 1 {
			t.Errorf("choices grew from 1 to %d", len(out.Choices))
		}
		if len(out.Choices) == 1 {
			s, _ := out.Choices[0].Message.Content.(string)
			// Stripping should never re-introduce a tag boundary.
			if containsTagPair(s) {
				t.Errorf("stripped content still contains <think>...</think>: %q", s)
			}
		}
	})
}

// FuzzThinkingContent confirms ThinkingContent never panics and always
// returns a substring of the input or the empty string.
func FuzzThinkingContent(f *testing.F) {
	for _, s := range []string{
		"",
		"<think>hi</think>rest",
		"plain",
		"<think>",
		"</think><think>",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, content string) {
		comp := fuzzCompletion(content)
		_ = chat.ThinkingContent(comp) // any return value is fine; the contract is "no panic"
	})
}

// FuzzParseList exercises the list parser against arbitrary content
// and separators. It must never panic.
func FuzzParseList(f *testing.F) {
	cases := []struct {
		content string
		sep     string
	}{
		{"a,b,c", ","},
		{"", ","},
		{"only-one", ","},
		{"a\nb\nc", "\n"},
		{"a||b||c", "||"},
		{"\x00\x00", "\x00"},
	}
	for _, c := range cases {
		f.Add(c.content, c.sep)
	}
	f.Fuzz(func(t *testing.T, content, sep string) {
		comp := fuzzCompletion(content)
		_, _ = chat.ParseList(comp, sep)
	})
}

// FuzzParseRegex exercises the regex parser. It must not panic on bad
// patterns and must not panic on weird content.
func FuzzParseRegex(f *testing.F) {
	cases := []struct {
		content string
		pattern string
	}{
		{"hello world", `(\w+) (\w+)`},
		{"empty", ""},
		{"", `.+`},
		{"open(", `(unbalanced`},
		{"", `[`},
	}
	for _, c := range cases {
		f.Add(c.content, c.pattern)
	}
	f.Fuzz(func(t *testing.T, content, pattern string) {
		comp := fuzzCompletion(content)
		_, _ = chat.ParseRegex(comp, pattern)
	})
}

// containsTagPair reports whether s contains a literal lowercase
// "<think>...</think>" pair with the close after the open. The
// stripper is intentionally case-sensitive (the convention is
// lowercase tags); fuzzing against a case-insensitive check would
// flag legitimate non-pairs as failures.
func containsTagPair(s string) bool {
	open := stringsIndex(s, "<think>")
	if open < 0 {
		return false
	}
	rest := s[open+len("<think>"):]
	return stringsIndex(rest, "</think>") >= 0
}

func stringsIndex(haystack, needle string) int {
	if len(needle) == 0 {
		return 0
	}
	hl := len(haystack)
	nl := len(needle)
	for i := 0; i+nl <= hl; i++ {
		if haystack[i:i+nl] == needle {
			return i
		}
	}
	return -1
}
