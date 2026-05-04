package chat_test

import (
	"testing"

	"github.com/buckedunicorn/grok/chat"
)

func TestThinkingContent_present(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{Role: "assistant", Content: "<think>I should reason first.</think>The answer is 42."},
		}},
	}
	got := chat.ThinkingContent(comp)
	if got != "I should reason first." {
		t.Errorf("unexpected thinking: %q", got)
	}
}

func TestThinkingContent_absent(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{Role: "assistant", Content: "Just an answer."},
		}},
	}
	if got := chat.ThinkingContent(comp); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestThinkingContent_noChoices(t *testing.T) {
	if got := chat.ThinkingContent(&chat.Completion{}); got != "" {
		t.Errorf("expected empty for no choices, got %q", got)
	}
}

func TestStripThinking_removesTag(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{Role: "assistant", Content: "<think>chain of thought</think>Final answer."},
		}},
	}
	stripped := chat.StripThinking(comp)
	content, _ := stripped.Choices[0].Message.Content.(string)
	if content != "Final answer." {
		t.Errorf("unexpected stripped content: %q", content)
	}
}

func TestStripThinking_noTag_returnsOriginal(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{Role: "assistant", Content: "No thinking here."},
		}},
	}
	stripped := chat.StripThinking(comp)
	if stripped != comp {
		t.Error("expected same pointer when no thinking tag present")
	}
}

func TestStripThinking_doesNotMutateOriginal(t *testing.T) {
	original := "<think>hidden</think>visible"
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{Role: "assistant", Content: original},
		}},
	}
	_ = chat.StripThinking(comp)
	if comp.Choices[0].Message.Content != original {
		t.Error("StripThinking must not mutate the original completion")
	}
}

func TestThinkingContent_reasoningContentField(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{
				Role:             "assistant",
				Content:          "The answer is 42.",
				ReasoningContent: "Step by step: I considered options and concluded 42.",
			},
		}},
	}
	got := chat.ThinkingContent(comp)
	if got != "Step by step: I considered options and concluded 42." {
		t.Errorf("expected reasoning_content, got %q", got)
	}
}

func TestThinkingContent_reasoningContentPreferredOverInlineTag(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{
				Role:             "assistant",
				Content:          "<think>inline noise</think>final",
				ReasoningContent: "structured reasoning",
			},
		}},
	}
	got := chat.ThinkingContent(comp)
	if got != "structured reasoning" {
		t.Errorf("reasoning_content should win, got %q", got)
	}
}

func TestStripThinking_clearsReasoningContent(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{
				Role:             "assistant",
				Content:          "Final answer.",
				ReasoningContent: "secret reasoning",
			},
		}},
	}
	stripped := chat.StripThinking(comp)
	if stripped.Choices[0].Message.ReasoningContent != "" {
		t.Errorf("expected ReasoningContent cleared, got %q", stripped.Choices[0].Message.ReasoningContent)
	}
	if stripped.Choices[0].Message.Content != "Final answer." {
		t.Errorf("Content should be untouched, got %v", stripped.Choices[0].Message.Content)
	}
}

func TestStripThinking_clearsBothReasoningAndInlineTag(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{
				Role:             "assistant",
				Content:          "<think>inline</think>final",
				ReasoningContent: "structured",
			},
		}},
	}
	stripped := chat.StripThinking(comp)
	if stripped.Choices[0].Message.ReasoningContent != "" {
		t.Error("ReasoningContent not cleared")
	}
	if stripped.Choices[0].Message.Content != "final" {
		t.Errorf("inline <think> not stripped, got %v", stripped.Choices[0].Message.Content)
	}
}

func TestStripThinking_doesNotMutateOriginalReasoningField(t *testing.T) {
	comp := &chat.Completion{
		Choices: []chat.Choice{{
			Message: chat.Message{
				Role:             "assistant",
				Content:          "x",
				ReasoningContent: "should-survive-on-original",
			},
		}},
	}
	_ = chat.StripThinking(comp)
	if comp.Choices[0].Message.ReasoningContent != "should-survive-on-original" {
		t.Error("StripThinking must not mutate the original ReasoningContent")
	}
}
