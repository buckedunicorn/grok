package chat

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrEmptyCompletion is returned by parsers when the Completion has no
// text content to parse.
var ErrEmptyCompletion = errors.New("chat: completion has no text content")

// firstText extracts the first choice's text content as a string. Returns
// ErrEmptyCompletion if the completion has no choices or non-string content.
func firstText(comp *Completion) (string, error) {
	if comp == nil || len(comp.Choices) == 0 {
		return "", ErrEmptyCompletion
	}
	s, ok := comp.Choices[0].Message.Content.(string)
	if !ok {
		return "", fmt.Errorf("chat: completion content is not a string (got %T)", comp.Choices[0].Message.Content)
	}
	return s, nil
}

// ParseList splits the completion content by sep and trims whitespace from
// each element. Empty elements are dropped. With sep == "" the default is
// "\n" (one item per line).
//
// Use as a standalone helper or wrap with runnable.Func[*chat.Completion, []string]
// to compose in a pipeline.
func ParseList(comp *Completion, sep string) ([]string, error) {
	text, err := firstText(comp)
	if err != nil {
		return nil, err
	}
	if sep == "" {
		sep = "\n"
	}
	parts := strings.Split(text, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// ParseRegex matches pattern against the completion content and returns
// the first match. If pattern has a capture group, the first group is
// returned; otherwise the whole match is returned. Returns an error if the
// pattern is invalid or no match is found.
func ParseRegex(comp *Completion, pattern string) (string, error) {
	text, err := firstText(comp)
	if err != nil {
		return "", err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("chat: compile regex: %w", err)
	}
	m := re.FindStringSubmatch(text)
	if m == nil {
		return "", fmt.Errorf("chat: regex %q did not match completion content", pattern)
	}
	if len(m) > 1 {
		return m[1], nil
	}
	return m[0], nil
}
