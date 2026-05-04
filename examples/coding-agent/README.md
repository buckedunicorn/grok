# coding-agent

A sandboxed coding agent: the `harness` gives it filesystem and shell tools, `sandbox.DockerExecutor` runs every shell command inside an ephemeral `python:3.12-alpine` container with the workspace mounted read-write.

## Run

```sh
docker pull python:3.12-alpine
XAI_API_KEY=... go run ./examples/coding-agent
```

The example prints the workspace path before the run; afterwards you can `cat $WORKSPACE/solution.py` and see exactly what the agent wrote.

## What it shows

- **Filesystem and shell point at the same directory.** `harness.NewLocalFS(workspace)` lets the agent write source files visible to the host user; `sandbox.DockerExecutor` mounts the same directory at `/workspace` inside the container, so `python3 solution.py` reads the just-written file. Two views of one workspace.
- **Hardening defaults.** `NetworkOff: true`, `MemoryMB: 256`, `CPUs: 1.0`, `User: <host uid:gid>`. The agent can't reach the network, can't fork-bomb, and any file it writes is owned by the host user (no `sudo` to clean up afterwards).
- **Failure-fast bootstrap.** `exec.Available()` runs `docker version` at startup so a missing/broken Docker is reported with a clean error before the run starts.

## Why python:3.12-alpine

Smallest mainstream image with a working Python interpreter (~50 MB). Any image with `python3` works, swap to `python:3.12-slim` for a Debian base, `python:3.12` for the full image, or a custom image if you need extra packages baked in.

## Sandbox semantics

Every `execute` tool call spawns a fresh `docker run --rm`, no state survives between calls beyond what's written to the mounted workspace. That means the agent can't `pip install` once and reuse it across calls (each container starts clean). For workflows that need persistent state, either:

- Bake dependencies into a custom image, or
- Pre-create a volume and mount it via `MountsRW`, or
- Use a long-running sandbox abstraction (out of scope for this example).
