# Microsoft Agent Framework with xAI Grok

[`microsoft/agent-framework`](https://github.com/microsoft/agent-framework), Microsoft's multi-language (Python + .NET) framework for building, orchestrating, and deploying AI agents. Successor to Semantic Kernel and AutoGen.

## Caveats

There is **no first-party xAI provider** in Microsoft Agent Framework as of late 2026. The recipes below use the framework's OpenAI provider with a base-URL override, this works for chat completions, but you should:

1. Verify your installed `agent_framework` version exposes a base-URL parameter on `OpenAIChatClient`. The shape varies by release.
2. Avoid framework features that depend on Azure-specific or OpenAI-Responses-specific server-side capabilities (Foundry projects, hosted tools).

If your installed version doesn't support a base-URL override, the cleanest path is a thin custom `ChatClientProtocol` implementation that wraps grok (Go) or `httpx` (Python), but that's beyond the scope of this recipe.

## Setup (Python)

```sh
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export XAI_API_KEY=...
```

## Recipe (Python)

```python
import asyncio
import os

from agent_framework import Agent
from agent_framework.openai import OpenAIChatClient


async def main():
    agent = Agent(
        client=OpenAIChatClient(
            api_key=os.environ["XAI_API_KEY"],
            base_url="https://api.x.ai/v1",
            model="grok-4-1-fast-reasoning",
        ),
        name="HaikuBot",
        instructions="You are an upbeat assistant that writes beautifully.",
    )

    print(await agent.run("Write a haiku about Microsoft Agent Framework."))


if __name__ == "__main__":
    asyncio.run(main())
```

If `OpenAIChatClient` in your installed version doesn't accept a `base_url`, fall back to:

```python
from openai import AsyncOpenAI
from agent_framework.openai import OpenAIChatClient

raw = AsyncOpenAI(base_url="https://api.x.ai/v1", api_key=os.environ["XAI_API_KEY"])
client = OpenAIChatClient(client=raw, model="grok-4-1-fast-reasoning")
```

## Recipe (.NET)

```csharp
using OpenAI;

var openai = new OpenAIClient(
    credential: new System.ClientModel.ApiKeyCredential(Environment.GetEnvironmentVariable("XAI_API_KEY")!),
    options: new OpenAIClientOptions { Endpoint = new Uri("https://api.x.ai/v1") });

var agent = openai
    .GetChatClient("grok-4-1-fast-reasoning")
    .AsAIAgent(name: "HaikuBot", instructions: "You are an upbeat assistant that writes beautifully.");

Console.WriteLine(await agent.RunAsync("Write a haiku about Microsoft Agent Framework."));
```

## Notes

- The framework's graph workflows, middleware, and OpenTelemetry tracing all work, they're orthogonal to the model provider.
- `DevUI` is a separate package and works with any provider; no special config needed for xAI.
- `Agents-as-tools` and handoffs run on the Python/.NET side, sending standard OpenAI-shaped requests to xAI.
