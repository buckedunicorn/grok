package harness

// DefaultInstructions is the system prompt the default Harness wires onto
// the Agent. Override with WithInstructions when you need a different
// persona or task framing.
//
// The prompt is intentionally short, every line earns its keep:
//
//  1. Plan-then-execute instruction primes the model to use write_todos
//     before doing work, which produces clearer trajectories.
//  2. Explicit tool-list pointer reminds the model that filesystem and
//     shell are scoped tools, not arbitrary side effects.
//  3. Final-answer guidance reduces models that ramble after the work is done.
const DefaultInstructions = `You are a careful, methodical assistant.

When given a non-trivial task:
1. Use write_todos to plan the steps before you start.
2. Use the available tools to make progress. Update the todo list as you go.
3. When everything is done, return a concise final answer to the user.

Be honest about uncertainty. If a tool call fails, surface the error rather
than guessing. Prefer many small tool calls over a single complex one.`
