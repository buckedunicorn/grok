# LangGraph with xAI Grok

[`langchain-ai/langgraph`](https://github.com/langchain-ai/langgraph), low-level orchestration framework for stateful, long-running agents. Consumes any LangChain chat model.

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
from langchain_core.messages import HumanMessage
from langgraph.prebuilt import create_react_agent

model = init_chat_model(
    model="grok-4-1-fast-reasoning",
    model_provider="openai",
    base_url="https://api.x.ai/v1",
    api_key=os.environ["XAI_API_KEY"],
)


def get_weather(city: str) -> str:
    """Returns the current temperature in Celsius for a city."""
    return f"{city}: 22 C, sunny"


graph = create_react_agent(
    model=model,
    tools=[get_weather],
    prompt="You are a concise assistant.",
)

result = graph.invoke({"messages": [HumanMessage("What's the weather in Tokyo?")]})
print(result["messages"][-1].content)
```

## Notes

- LangGraph's prebuilt `create_react_agent` is the simplest entry point; for full graph control, build a `StateGraph` and add the chat model as a node, same `init_chat_model(...)` line.
- LangGraph's durable-execution and human-in-the-loop features depend on a checkpointer (e.g. `MemorySaver`, Postgres) and don't depend on the model provider.
- Streaming via `graph.stream(...)` and `graph.astream(...)` works the same way as with any LangChain chat model.
