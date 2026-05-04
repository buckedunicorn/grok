# agents/sandbox

Sandboxing implementations of the [`harness.Executor`](../harness) contract. Drop one of these into a `harness` to run shell commands inside an isolated environment instead of on the host.

```go
import "github.com/buckedunicorn/grok/agents/sandbox"
```

## Why shell out instead of using a Go SDK

The package shells out to vendor CLIs (`docker` today; `firecracker-ctl` later if demand warrants) instead of importing their Go SDKs. That keeps grok's dependency tree free of heavy transitive dependencies. Users need the vendor binary on `$PATH`; that is the only requirement.

## Surface

| Type | Purpose |
|---|---|
| `DockerExecutor` | Runs every command inside an ephemeral `docker run --rm` container. Configurable image, mounts, env, network-off, memory, CPU, user |

## Example

```go
exec := &sandbox.DockerExecutor{
    Image:      "python:3.12-alpine",
    Workdir:    "/workspace",
    MountsRW:   map[string]string{workspaceDir: "/workspace"},
    NetworkOff: true,
    MemoryMB:   256,
    CPUs:       1.0,
    User:       "1000:1000",
}
if err := exec.Available(); err != nil {
    return fmt.Errorf("docker: %w", err)
}

h := harness.NewDefault(client.Chat, "grok-4-1-fast-reasoning",
    harness.WithExecutor(exec),
)
```

`exec.Available()` checks that the docker binary is installed and callable. Call it at startup to fail fast on misconfigured environments.

## Examples directory

- [`examples/sandbox`](../../examples/sandbox)
- [`examples/sandboxed-eval`](../../examples/sandboxed-eval)
- [`examples/full-stack`](../../examples/full-stack)
