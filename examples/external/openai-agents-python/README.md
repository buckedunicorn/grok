# openai-agents-python with xAI Grok

[`openai/openai-agents-python`](https://github.com/openai/openai-agents-python), OpenAI's lightweight multi-agent SDK. Works with any OpenAI-compatible endpoint by passing a customized `AsyncOpenAI` client into `OpenAIChatCompletionsModel`.

## Setup

```sh
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export XAI_API_KEY=...
```

## Recipe

```python
import asyncio
import os

from agents import Agent, Runner
from agents.models.openai_chatcompletions import OpenAIChatCompletionsModel
from openai import AsyncOpenAI


async def main():
    client = AsyncOpenAI(
        base_url="https://api.x.ai/v1",
        api_key=os.environ["XAI_API_KEY"],
    )

    agent = Agent(
        name="Helper",
        instructions="You are a concise assistant. Always answer in one sentence.",
        model=OpenAIChatCompletionsModel(
            model="grok-4-1-fast-reasoning",
            openai_client=client,
        ),
    )

    result = await Runner.run(agent, "What's the fastest sorting algorithm for nearly-sorted data?")
    print(result.final_output)


if __name__ == "__main__":
    asyncio.run(main())
```

## Notes

- Use `OpenAIChatCompletionsModel` (chat completions) **not** `OpenAIResponsesModel`. xAI's `/v1/responses` endpoint is OpenAI-compatible only at the request shape level; it doesn't expose OpenAI's hosted tools (`web_search`, `file_search`, MCP server URLs, computer use). The Chat Completions path is the safe default.
- Tool calling, handoffs, sessions, and guardrails all work, they're driven by the SDK in Python, with Grok as the underlying chat model.
- For the Hermes-format tools openai-agents-python supports natively, just register them as Python `@function_tool` decorated functions; the model produces standard OpenAI-shaped tool calls and the SDK dispatches them.
