# Hermes Agent with xAI Grok

[`NousResearch/hermes-agent`](https://github.com/NousResearch/hermes-agent), Nous Research's self-improving end-user agent: terminal TUI, messaging gateway, closed learning loop, MCP support. Configurable to use any OpenAI-compatible endpoint at runtime, switch with `hermes model`, no code changes.

## Setup

```sh
curl -fsSL https://raw.githubusercontent.com/NousResearch/hermes-agent/main/scripts/install.sh | bash
source ~/.bashrc      # or ~/.zshrc
export XAI_API_KEY=...
```

## Recipe

The exact CLI surface for setting an OpenAI-compatible provider varies by Hermes version. Run `hermes config show` to see the current schema; the canonical interactive setup is `hermes setup`. The configuration is durable, you only do it once.

Most reliable path: configure interactively.

```sh
hermes model
# Choose: "OpenAI compatible (custom endpoint)"
# When prompted:
#   Base URL: https://api.x.ai/v1
#   API key:  $XAI_API_KEY
#   Model:    grok-4-1-fast-reasoning
hermes        # start chatting
```

Direct config edits (verify key paths against `hermes config show` for your version):

```sh
hermes config set provider openai_compat
hermes config set provider.openai_compat.base_url "https://api.x.ai/v1"
hermes config set provider.openai_compat.api_key "$XAI_API_KEY"
hermes config set provider.openai_compat.model "grok-4-1-fast-reasoning"
```

## Verify

```sh
hermes doctor
```

Should report the configured provider, model, and a successful health-check round trip.

## Notes

- Hermes was originally trained on the NousResearch tool-call format, but its harness translates internally, Grok's native function-calling format works without manual prompt rewriting.
- Voice memos, the messaging gateway (Telegram / Discord / Slack / WhatsApp / Signal / Email), cron scheduler, and skills system are all client-side features; they work with any model the provider config points to.
- MCP integration on the Hermes side connects MCP **servers** to the agent, orthogonal to which LLM provider serves the chat completions. xAI does not need to be MCP-aware for this to work.
- For the OpenClaw → Hermes migration path, see `hermes claw migrate --help`.
