package chat_test

import (
	"testing"

	"github.com/buckedunicorn/grok/chat"
)

type sampleOutput struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

func TestDecode_success(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{
			{Message: chat.Message{Role: "assistant", Content: `{"name":"grok","score":42}`}},
		},
	}
	out, err := chat.Decode[sampleOutput](comp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "grok" || out.Score != 42 {
		t.Errorf("unexpected output: %+v", out)
	}
}

func TestDecode_noChoices(t *testing.T) {
	_, err := chat.Decode[sampleOutput](&chat.Completion{})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestDecode_contentNotString(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{
			{Message: chat.Message{Role: "assistant", Content: []string{"not", "a", "string"}}},
		},
	}
	_, err := chat.Decode[sampleOutput](comp)
	if err == nil {
		t.Fatal("expected error for non-string content")
	}
}

func TestDecode_invalidJSON(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{
			{Message: chat.Message{Role: "assistant", Content: `not json`}},
		},
	}
	_, err := chat.Decode[sampleOutput](comp)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// regression: Decode must not panic on nil.
func TestDecode_nilCompletionReturnsError(t *testing.T) {
	_, err := chat.Decode[sampleOutput](nil)
	if err == nil {
		t.Fatal("expected error from Decode(nil), got nil")
	}
}
