package chat

import (
	"regexp"
	"strings"
)

var thinkRE = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

// ThinkingContent extracts the model's chain-of-thought from a Completion.
//
// Two pathways are supported:
//
//  1. The dedicated `reasoning_content` field (xAI reasoning models, OpenAI
//     o1-style providers, DeepSeek-R1, etc.). When present, this is preferred.
//  2. Inline `<think>...</think>` tags inside the message content (older or
//     simpler models that emit reasoning into the visible response).
//
// Returns "" if neither pathway carries reasoning text. Callers that only
// care whether *any* reasoning happened, without needing the trace itself  -
// should look at comp.Usage.CompletionTokensDetails.ReasoningTokens instead;
// that counter is reliable even when the trace is hidden from the response.
func ThinkingContent(comp *Completion) string {
	if comp == nil || len(comp.Choices) == 0 {
		return ""
	}
	msg := comp.Choices[0].Message
	if rc := strings.TrimSpace(msg.ReasoningContent); rc != "" {
		return rc
	}
	content, ok := msg.Content.(string)
	if !ok {
		return ""
	}
	m := thinkRE.FindStringSubmatch(content)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// StripThinking returns a shallow copy of comp with chain-of-thought removed
// from the first choice. Both pathways are stripped: the `reasoning_content`
// field is cleared, and inline `<think>...</think>` tags are deleted from
// the content. Returns the original pointer when nothing needed stripping.
func StripThinking(comp *Completion) *Completion {
	if comp == nil || len(comp.Choices) == 0 {
		return comp
	}
	msg := comp.Choices[0].Message
	hasReasoning := msg.ReasoningContent != ""

	contentStr, _ := msg.Content.(string)
	stripped := thinkRE.ReplaceAllString(contentStr, "")
	if stripped != contentStr {
		stripped = strings.TrimSpace(stripped)
	}
	contentChanged := stripped != contentStr

	if !hasReasoning && !contentChanged {
		return comp
	}

	out := *comp
	choices := make([]Choice, len(comp.Choices))
	copy(choices, comp.Choices)
	choices[0].Message.ReasoningContent = ""
	if contentChanged {
		choices[0].Message.Content = stripped
	}
	out.Choices = choices
	return &out
}
