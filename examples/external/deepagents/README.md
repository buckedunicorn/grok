# deepagents with xAI Grok

[`langchain-ai/deepagents`](https://github.com/langchain-ai/deepagents), LangChain's batteries-included harness on top of LangGraph: planning, filesystem tools, sub-agents, context management. Provider-agnostic; consumes any LangChain chat model.

## Setup

```sh
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export XAI_API_KEY=...
```

## Recipe

```python
import os

from langchain.chat_models import init_chat_model
from deepagents import create_deep_agent

model = init_chat_model(
    model="grok-4-1-fast-reasoning",
    model_provider="openai",
    base_url="https://api.x.ai/v1",
    api_key=os.environ["XAI_API_KEY"],
)

agent = create_deep_agent(
    model=model,
    system_prompt="You are a concise research assistant.",
)

result = agent.invoke({
    "messages": [
        {"role": "user", "content": "Research the grok SDK and write a one-paragraph summary."},
    ]
})
print(result["messages"][-1].content)
```

## Notes

- `model_provider="openai"` plus `base_url` routes to xAI's OpenAI-compatible chat completions endpoint.
- DeepAgents' default toolset (planning + filesystem + shell + sub-agents) all work over Grok, they're driven on the Python side and only require a chat model with tool-calling support.
- For sandboxed shell execution, follow DeepAgents' own sandbox docs, it doesn't depend on the model provider.
- The agent returns a compiled LangGraph graph. Streaming, checkpointing, and LangSmith tracing all work the same way as with any other model.
