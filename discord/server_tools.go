package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/buckedunicorn/grok/chat"
	"github.com/buckedunicorn/grok/responses"
)

// Default model + timeout for server-side-tools subcalls. The chat
// loop runs on whatever model the Agent is configured with (often a
// non-reasoning model for snappy turns); these subcalls deliberately
// use a reasoning model since they have to plan over fresh data or
// multi-step compute.
const (
	DefaultServerSideModel   = "grok-4-1-fast-reasoning"
	DefaultServerSideTimeout = 75 * time.Second
)

// ServerToolOption customises a server-side tool factory
// (WebSearchTool, RunCodeTool). The same option type is shared because
// the configurable surface is identical.
type ServerToolOption func(*serverToolConfig)

type serverToolConfig struct {
	model    string
	timeout  time.Duration
	ackTpl   string // "%s" → user-facing query/task; "" disables the ack
	logf     func(format string, args ...any)
	toolName string // override the function-tool name advertised to the model
}

// WithServerToolModel overrides the model used for the Responses API
// subcall. Defaults to DefaultServerSideModel.
func WithServerToolModel(model string) ServerToolOption {
	return func(c *serverToolConfig) { c.model = model }
}

// WithServerToolTimeout caps how long the Responses API subcall is
// allowed to run. Defaults to DefaultServerSideTimeout.
func WithServerToolTimeout(d time.Duration) ServerToolOption {
	return func(c *serverToolConfig) { c.timeout = d }
}

// WithServerToolAck overrides the channel-side ack template. The
// template should contain a single %s, which is replaced with a one-
// line preview of the query/task before posting. Pass "" to disable
// the ack entirely (useful when the calling code wants to manage
// progress messages itself).
func WithServerToolAck(tpl string) ServerToolOption {
	return func(c *serverToolConfig) { c.ackTpl = tpl }
}

// WithServerToolLogger plumbs an optional logger through the handler.
// If nil, the handler is silent. The format string is plain printf;
// arguments include channel ID, query/task, length, etc.
func WithServerToolLogger(logf func(format string, args ...any)) ServerToolOption {
	return func(c *serverToolConfig) { c.logf = logf }
}

// WithServerToolName overrides the function-tool name advertised to
// the model (defaults: "search_web" for WebSearchTool, "run_code" for
// RunCodeTool). Useful when you have multiple instances configured
// differently (e.g. a domain-restricted search).
func WithServerToolName(name string) ServerToolOption {
	return func(c *serverToolConfig) { c.toolName = name }
}

func mergeOptions(defaultName, defaultAck string, opts []ServerToolOption) *serverToolConfig {
	c := &serverToolConfig{
		model:    DefaultServerSideModel,
		timeout:  DefaultServerSideTimeout,
		ackTpl:   defaultAck,
		toolName: defaultName,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// WebSearchTool returns a (chat.Tool, chat.Handler) pair you can plug
// into AgentConfig{Tools, Handlers}. The handler:
//
// - reads the channel ID from ctx via ChannelFromContext (Agent.Handle
// populates it automatically);
// - posts a short "Searching the web for: <query>" ack via sender if
// non-nil and a channel ID is set;
// - calls grok's Responses API with web_search + x_search server-side
// tools enabled;
// - returns the synthesized answer (with inline citation markdown) to
// the chat loop, plus tool-invocation/source counts for observability.
//
// Pass sender = nil to skip ack messages, useful for non-Discord
// callers or when the surrounding code shows progress its own way.
//
// Why a separate Responses subcall instead of Live Search on the chat
// request? Live Search on /v1/chat/completions is deprecated; web_search
// and x_search are Responses-API-only. As an explicit function tool,
// the chat agent stays on chat.RunAgent (no need to migrate the whole
// conversational loop) and every search query is observable via the
// optional logger.
func WebSearchTool(client *responses.Client, sender Sender, opts ...ServerToolOption) (chat.Tool, chat.Handler) {
	cfg := mergeOptions("search_web", "🔍 *Searching the web for:* %s", opts)
	tool := chat.Tool{
		Type: "function",
		Function: chat.FunctionDef{
			Name:        cfg.toolName,
			Description: `Search the live web (and X / Twitter posts) for current information that's outside the model's training data, news, recent events, prices, schedules, recent releases, "what is X saying right now". Returns a synthesized answer with markdown-link citations. Use this whenever the question depends on up-to-date or post-training-cutoff information; do NOT use for math / general knowledge / things you already know.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query. Phrase it as the question you want answered. The remote search agent will plan its own keywords.",
					},
				},
				"required": []string{"query"},
			},
		},
	}
	handler := serverToolHandler(client, sender, cfg, "query", []responses.Tool{
		{Type: "web_search"},
		{Type: "x_search"},
	})
	return tool, handler
}

// RunCodeTool returns a (chat.Tool, chat.Handler) pair that wraps xAI's
// `code_interpreter` server-side tool. The user (model) describes a
// task in plain English; the remote agent writes and runs Python in a
// sandbox; the synthesized answer comes back to the chat loop.
//
// Channel ID + sender behavior is identical to WebSearchTool. Pass
// sender = nil to skip the "Running Python: <task>" ack.
func RunCodeTool(client *responses.Client, sender Sender, opts ...ServerToolOption) (chat.Tool, chat.Handler) {
	cfg := mergeOptions("run_code", "🐍 *Running Python:* %s", opts)
	tool := chat.Tool{
		Type: "function",
		Function: chat.FunctionDef{
			Name:        cfg.toolName,
			Description: `Run Python in a sandboxed runtime to compute something exactly. Use for: nontrivial math, date/time arithmetic, base / hex / unit conversion, regex against a sample string, parsing or summarizing a small block of structured data the user pasted, anything where a deterministic computation beats a model guess. The remote agent writes the code itself, describe the task. Do NOT use for casual chat or things you can answer directly.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task": map[string]any{
						"type":        "string",
						"description": "Plain-English description of what should be computed. Include any concrete inputs the user gave (numbers, strings, sample text). The remote agent decides the implementation.",
					},
				},
				"required": []string{"task"},
			},
		},
	}
	handler := serverToolHandler(client, sender, cfg, "task", []responses.Tool{
		{Type: "code_interpreter"},
	})
	return tool, handler
}

// serverToolHandler wires the shared body of the server-side tool
// handlers (search_web, run_code, …), argument parsing, optional ack,
// Responses subcall, logging, and JSON-formatted return. argField names
// the single string field the function tool exposes ("query" / "task").
func serverToolHandler(client *responses.Client, sender Sender, cfg *serverToolConfig, argField string, tools []responses.Tool) chat.Handler {
	return func(ctx context.Context, args string) (string, error) {
		var raw map[string]any
		if err := json.Unmarshal([]byte(args), &raw); err != nil {
			return "", fmt.Errorf("%s: %w", cfg.toolName, err)
		}
		query, _ := raw[argField].(string)
		query = strings.TrimSpace(query)
		if query == "" {
			return jsonErr(argField + " is required"), nil
		}

		channelID := ChannelFromContext(ctx)
		if cfg.logf != nil {
			cfg.logf("[%s] tool %s: %s=%q", short(channelID), cfg.toolName, argField, truncateOneLine(query, 120))
		}

		// Optional channel ack.
		if sender != nil && channelID != "" && cfg.ackTpl != "" {
			ack := fmt.Sprintf(cfg.ackTpl, truncateOneLine(query, 80))
			if err := sender.Send(channelID, ack); err != nil && cfg.logf != nil {
				cfg.logf("[%s] tool %s: ack send error: %v", short(channelID), cfg.toolName, err)
			}
		}

		subCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
		defer cancel()

		resp, err := client.Create(subCtx, &responses.CreateRequest{
			Model: cfg.model,
			Input: query,
			Tools: tools,
		})
		if err != nil {
			if cfg.logf != nil {
				cfg.logf("[%s] tool %s: error: %v", short(channelID), cfg.toolName, err)
			}
			return jsonErr(err.Error()), nil
		}

		text := resp.OutputText()
		if text == "" {
			if cfg.logf != nil {
				cfg.logf("[%s] tool %s: empty output", short(channelID), cfg.toolName)
			}
			return jsonErr(cfg.toolName + " returned no answer"), nil
		}

		toolsUsed, sources := 0, 0
		if resp.Usage != nil {
			toolsUsed = resp.Usage.NumServerSideToolsUsed
			sources = resp.Usage.NumSourcesUsed
		}
		if cfg.logf != nil {
			cfg.logf("[%s] tool %s: ok len=%d tools_used=%d sources=%d",
				short(channelID), cfg.toolName, len(text), toolsUsed, sources)
		}

		out, _ := json.Marshal(serverToolOut{
			Answer:          text,
			ToolInvocations: toolsUsed,
			SourcesUsed:     sources,
		})
		return string(out), nil
	}
}

// serverToolOut is the JSON shape returned by WebSearchTool and
// RunCodeTool. Typed struct skips the per-call map[string]any
// allocation.
type serverToolOut struct {
	Answer          string `json:"answer"`
	ToolInvocations int    `json:"tool_invocations"`
	SourcesUsed     int    `json:"sources_used"`
}

// jsonErr formats a tool-error message as a JSON object. Returned as
// the tool result so the model can react to a failure mid-turn rather
// than aborting. Typed struct path.
func jsonErr(msg string) string {
	out, _ := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: msg})
	return string(out)
}

// short returns the first 8 chars of an id for compact log lines.
// Mirrors the helper used by the discord-bot example, lifted here so
// the logger format strings stay terse without exporting more API.
func short(id string) string {
	const n = 8
	if len(id) <= n {
		return id
	}
	return id[:n]
}

// truncateOneLine collapses whitespace runs to single spaces and caps
// the result at max characters with an ellipsis. Used for both log
// lines and channel-side ack labels.
func truncateOneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
