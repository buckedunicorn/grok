// Package council provides multi-agent roundtable and synthesis patterns built
// on top of the chat completions API.
package council

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/buckedunicorn/grok/chat"
)

// Participant is a named agent in a council session.
type Participant struct {
	Name   string
	Client *chat.Client
	Model  string
	// System is an optional system prompt injected per-request but not stored
	// in the conversation history.
	System string
}

// Response holds one participant's answer to the council question.
type Response struct {
	Name   string
	Output string
	Usage  chat.Usage
	// Err is non-nil when this participant's API call failed. Other
	// participants' results are still returned.
	Err error
}

// Roundtable asks question to all participants concurrently and returns their
// responses in the same order as participants. Context cancellation or a
// deadline propagates to all in-flight requests and returns an error.
// Individual API errors are captured in Response.Err rather than aborting the
// whole call.
func Roundtable(ctx context.Context, question string, participants []Participant) ([]Response, error) {
	responses := make([]Response, len(participants))
	g, gctx := errgroup.WithContext(ctx)

	for i, p := range participants {
		i, p := i, p
		g.Go(func() error {
			msgs := buildMessages(p.System, question)
			comp, err := p.Client.Create(gctx, &chat.CreateRequest{
				Model:    p.Model,
				Messages: msgs,
			})
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				responses[i] = Response{Name: p.Name, Err: err}
				return nil
			}
			responses[i] = Response{
				Name:   p.Name,
				Output: firstContent(comp),
				Usage:  comp.Usage,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return responses, nil
}

// Synthesize runs Roundtable and then asks moderator to synthesize all
// responses into a single coherent answer.
func Synthesize(ctx context.Context, question string, participants []Participant, moderator Participant) (string, error) {
	responses, err := Roundtable(ctx, question, participants)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "The following agents were asked: %q\n\n", question)
	for _, r := range responses {
		if r.Err != nil {
			fmt.Fprintf(&sb, "**%s**: (error: %s)\n\n", r.Name, r.Err)
		} else {
			fmt.Fprintf(&sb, "**%s**: %s\n\n", r.Name, r.Output)
		}
	}
	sb.WriteString("Synthesize these responses into a single coherent answer.")

	msgs := buildMessages(moderator.System, sb.String())
	comp, err := moderator.Client.Create(ctx, &chat.CreateRequest{
		Model:    moderator.Model,
		Messages: msgs,
	})
	if err != nil {
		return "", fmt.Errorf("grok/council: synthesizer: %w", err)
	}
	if len(comp.Choices) == 0 {
		return "", errors.New("grok/council: synthesizer returned no choices")
	}
	return firstContent(comp), nil
}

func buildMessages(system, userContent string) []chat.Message {
	msgs := make([]chat.Message, 0, 2)
	if system != "" {
		msgs = append(msgs, chat.Message{Role: "system", Content: system})
	}
	msgs = append(msgs, chat.Message{Role: "user", Content: userContent})
	return msgs
}

func firstContent(comp *chat.Completion) string {
	if len(comp.Choices) == 0 {
		return ""
	}
	s, _ := comp.Choices[0].Message.Content.(string)
	return s
}
