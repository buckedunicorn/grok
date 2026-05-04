package chat

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Decode extracts the first choice's text content from c and JSON-unmarshals
// it into T. Use this with ResponseFormat{Type: "json_object"} or "json_schema"
// requests to get a typed result without a separate json.Unmarshal call.
func Decode[T any](c *Completion) (T, error) {
	var zero T
	if c == nil || len(c.Choices) == 0 {
		return zero, errors.New("grok: completion has no choices")
	}
	content, ok := c.Choices[0].Message.Content.(string)
	if !ok {
		return zero, errors.New("grok: message content is not a string")
	}
	var out T
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return zero, fmt.Errorf("grok: decoding completion content: %w", err)
	}
	return out, nil
}
