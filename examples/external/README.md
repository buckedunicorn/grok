# external, using Grok with non-Go agent frameworks

xAI's API is OpenAI-compatible, so most external agent frameworks (mostly Python and TypeScript) can talk to Grok by setting two values: a base URL and an API key. **No Go SDK glue is required or possible**, these frameworks don't run in Go. This directory is documentation only: one folder per framework with a working configuration recipe.

Set everywhere in this directory:

```sh
export XAI_API_KEY=...   # https://console.x.ai
```

The base URL is always `https://api.x.ai/v1`.

## Frameworks covered

| Framework | Language | What it is | Recipe |
|---|---|---|---|
| [openai-agents-python](./openai-agents-python/) | Python | OpenAI's lightweight multi-agent SDK (Agent / Runner / Handoffs / Sessions) | OpenAI client with `base_url` override |
| [deepagents](./deepagents/) | Python | LangChain's batteries-included harness on top of LangGraph | `init_chat_model` with OpenAI provider + base URL |
| [langgraph](./langgraph/) | Python | LangChain's low-level graph orchestration framework | Same as deepagents, both consume LangChain chat models |
| [microsoft-agent-framework](./microsoft-agent-framework/) | Python + .NET | Microsoft's multi-agent framework (successor to Semantic Kernel + AutoGen) | `OpenAIChatClient` with `base_url` (Python) or `OpenAIClient` ctor URI (.NET) |
| [hermes-agent](./hermes-agent/) | Python | Nous Research's self-improving end-user agent, CLI, messaging gateway, learning loop | `hermes config set` for the `openai_compat` provider |
| [openclaw](./openclaw/) | TypeScript | Personal-AI-assistant gateway with 20+ messaging channels | Models config with `baseURL` + `apiKey` |

## Recommended models

When the framework asks for a model name, pick one of:

| Model ID | Use for |
|---|---|
| `grok-4-1-fast-reasoning` | Tool use, multi-step agent loops, structured output, anything where reasoning matters |
| `grok-4-1-fast-non-reasoning` | Plain chat, summaries, throughput-bound workloads |

See https://docs.x.ai/developers/models for the full list and pricing.

## Caveats

- Recipes assume each framework's *current* configuration surface as of late 2026, APIs change, especially in fast-moving Python frameworks. Treat each recipe as a starting point, not a stable contract. The framework's own docs are authoritative.
- xAI's chat completions and responses APIs accept OpenAI-shaped requests but do **not** accept OpenAI Responses-API-specific server-side features like MCP server URLs, hosted tools (`code_interpreter`, `file_search`, `web_search` as OpenAI knows them), or computer-use tools. If a framework defaults to those, you'll need to disable them or use the OpenAI client path the framework provides for plain chat completions.
- These recipes are out-of-tree: the grok Go module has no dependency on any framework here.
