// Package prompt provides string templates with {name} variable
// substitution for chat messages.
//
// Templates are intentionally minimal, substitution only, no control flow.
// For conditionals or loops, use text/template directly; this package exists
// to make the common case (substituting a few variables into a system or
// user prompt) feel natural without pulling in a templating language.
//
// Example:
//
//	t := prompt.MustNew("Summarize the following in {style} style: {text}")
//	rendered, err := t.Render(map[string]any{"style": "bulleted", "text": "..."})
//
// ChatTemplate renders System/User pairs into []chat.Message ready for
// chat.CreateRequest:
//
//	ct := &prompt.ChatTemplate{
//	 System: "You are a {persona} assistant.",
//	 User: "{question}",
//	}
//	msgs, _ := ct.Render(map[string]any{"persona": "concise", "question": "What is Go?"})
package prompt

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/buckedunicorn/grok/chat"
)

// placeholderRE matches {name} where name is a valid identifier-style token.
// Whitespace inside the braces is allowed and trimmed: { name } parses as
// "name". Escape with double braces: {{ and }} render as literal { and }.
var placeholderRE = regexp.MustCompile(`\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}`)

// Template is a parsed string template with {name} placeholders.
type Template struct {
	text string
	vars []string // unique placeholder names in first-occurrence order
}

// New parses text and returns a Template. Returns an error if the text
// is malformed (currently always nil; reserved for future syntax).
func New(text string) (*Template, error) {
	t := &Template{text: text}
	seen := map[string]struct{}{}
	for _, m := range placeholderRE.FindAllStringSubmatch(text, -1) {
		name := m[1]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		t.vars = append(t.vars, name)
	}
	return t, nil
}

// MustNew parses text and panics on error. Useful for templates declared
// at package init time.
func MustNew(text string) *Template {
	t, err := New(text)
	if err != nil {
		panic(err)
	}
	return t
}

// Vars returns the placeholder names in first-occurrence order.
func (t *Template) Vars() []string {
	out := make([]string, len(t.vars))
	copy(out, t.vars)
	return out
}

// Render substitutes vars into the template and returns the result. Returns
// an error if any required placeholder is missing from vars.
func (t *Template) Render(vars map[string]any) (string, error) {
	missing := []string{}
	for _, name := range t.vars {
		if _, ok := vars[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("prompt: missing variables: %s", strings.Join(missing, ", "))
	}

	// Single-pass substitution: walk the regex matches with submatch
	// indices and splice into a strings.Builder. Avoids the second
	// regex pass that ReplaceAllStringFunc -> FindStringSubmatch would
	// trigger per match.
	matches := placeholderRE.FindAllStringSubmatchIndex(t.text, -1)
	if len(matches) == 0 {
		return t.text, nil
	}
	var sb strings.Builder
	sb.Grow(len(t.text))
	last := 0
	for _, m := range matches {
		// m[0:2] is the full match span; m[2:4] is the first capture
		// group (the placeholder name).
		sb.WriteString(t.text[last:m[0]])
		name := t.text[m[2]:m[3]]
		sb.WriteString(formatVar(vars[name]))
		last = m[1]
	}
	sb.WriteString(t.text[last:])
	return sb.String(), nil
}

// formatVar fast-paths the common case (string value) without going
// through fmt's reflection.
func formatVar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(v)
	}
}

// Partial binds a subset of vars and returns a new Template whose remaining
// placeholders are the unbound ones. Useful for currying templates that
// share common context.
func (t *Template) Partial(vars map[string]any) *Template {
	if len(vars) == 0 {
		return &Template{text: t.text, vars: append([]string(nil), t.vars...)}
	}
	rendered := placeholderRE.ReplaceAllStringFunc(t.text, func(match string) string {
		sub := placeholderRE.FindStringSubmatch(match)
		if v, ok := vars[sub[1]]; ok {
			return fmt.Sprint(v)
		}
		return match
	})
	out, _ := New(rendered)
	return out
}

// ChatTemplate renders a System/User pair into []chat.Message. Both fields
// share the same vars map at Render time. An empty System field is omitted
// from the output (no leading system message).
//
// The System and User strings are parsed once on first Render and the
// parsed forms are reused on subsequent calls. Mutating
// the strings after the first Render does not refresh the cache; use
// NewChatTemplate to build a frozen template, or assign a new
// *ChatTemplate value if you need a different prompt.
type ChatTemplate struct {
	System string
	User   string

	once       sync.Once
	systemTmpl *Template
	userTmpl   *Template
	parseErr   error
}

// NewChatTemplate parses System and User up front and returns a
// ChatTemplate ready for repeated Render calls. Returns an error if
// either template fails to parse.
func NewChatTemplate(system, user string) (*ChatTemplate, error) {
	ct := &ChatTemplate{System: system, User: user}
	ct.parse()
	if ct.parseErr != nil {
		return nil, ct.parseErr
	}
	return ct, nil
}

// parse compiles the System/User templates exactly once. Safe for
// concurrent first-callers; the sync.Once guarantees a single parse.
func (t *ChatTemplate) parse() {
	t.once.Do(func() {
		if t.System != "" {
			st, err := New(t.System)
			if err != nil {
				t.parseErr = fmt.Errorf("prompt: parse system: %w", err)
				return
			}
			t.systemTmpl = st
		}
		ut, err := New(t.User)
		if err != nil {
			t.parseErr = fmt.Errorf("prompt: parse user: %w", err)
			return
		}
		t.userTmpl = ut
	})
}

// Render substitutes vars into the System and User templates and returns
// the resulting message slice ready for chat.CreateRequest.
func (t *ChatTemplate) Render(vars map[string]any) ([]chat.Message, error) {
	t.parse()
	if t.parseErr != nil {
		return nil, t.parseErr
	}
	out := make([]chat.Message, 0, 2)
	if t.systemTmpl != nil {
		s, err := t.systemTmpl.Render(vars)
		if err != nil {
			return nil, fmt.Errorf("prompt: render system: %w", err)
		}
		out = append(out, chat.Message{Role: "system", Content: s})
	}
	u, err := t.userTmpl.Render(vars)
	if err != nil {
		return nil, fmt.Errorf("prompt: render user: %w", err)
	}
	out = append(out, chat.Message{Role: "user", Content: u})
	return out, nil
}
