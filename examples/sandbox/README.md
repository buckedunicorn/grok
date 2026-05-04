# sandbox

Wire `sandbox.DockerExecutor` into `harness.NewDefault` so the agent's `execute` shell tool runs inside an ephemeral Docker container instead of on the host.

## Run

```sh
# Make sure docker is installed and the alpine image is cached
docker pull alpine:3.20

XAI_API_KEY=... go run ./examples/sandbox
```

## What it shows

- `sandbox.DockerExecutor` satisfies `harness.Executor` structurally, drop it in via `harness.WithExecutor(...)` with no SDK changes
- Hardening defaults: `NetworkOff: true`, non-root `User`, `MemoryMB`, `CPUs` quotas, the agent gets the bare minimum it needs
- `exec.Available()` lets you fail fast at startup if docker isn't installed
- Each `execute` tool call spawns a fresh container (`docker run --rm`), so there's zero state leakage between calls

## When to reach for this vs. LocalExecutor

| Use | Executor |
|---|---|
| Local-developer workflows; you trust the model and want speed | `&harness.LocalExecutor{Workdir: "/tmp/agent"}` |
| Anything with untrusted prompts, tools, or model output landing on a shared host | `&sandbox.DockerExecutor{...}` |
| Stricter isolation than Docker (microVM) | A custom Executor that shells out to `firecracker-ctl` or `gvisor`'s `runsc`, same `Run(ctx, cmd, args, stdin)` contract |

## Why CLI-shellout instead of the Docker Go SDK

The Docker SDK (`github.com/docker/docker/client`) is heavy, pulls dozens of transitive dependencies into your binary. Shelling out to the `docker` CLI keeps `agents/sandbox` stdlib-only. Trade-off: requires the binary on PATH (most production systems already have it). If you need streaming logs, attach-to-running-container, or other docker-API features beyond what `docker run` exposes, write a custom Executor that uses the Go SDK directly, the contract is small enough that it's a one-file project.
