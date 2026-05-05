# Security Policy

Thank you for taking the time to make grok safer. This document describes how to report a vulnerability, what is in scope, and what to expect after a report.

## Reporting a vulnerability

Please report suspected vulnerabilities privately. Do not open a public GitHub issue for security reports.

- Email: <security@buckedunicorn.com>.

Include in your report:

- A clear description of the issue and the impact you observed.
- The commit or release version you tested against.
- A minimal reproduction (Go test, curl invocation, or short program) when possible.
- Whether you have already disclosed publicly or to other parties.

We acknowledge reports within 3 business days, share a remediation plan within 10 business days, and aim to ship a fix within 30 calendar days for HIGH-severity issues. Coordinated disclosure is preferred; we will agree a public advisory date with you before publishing.

## Scope

In scope:

- Code under this repository's library packages in the root module (`chat/`, `responses/`, `images/`, `videos/`, `voice/**`, `models/`, `files/`, `batches/`, `grpc/`, `agents/`, `agents/durable/`, `agents/eval/`, `agents/harness/`, `agents/sandbox/`, `discord/`, `prompt/`, `runnable/`, `workflow/`, `queue/`, `memory/`, `cache/`, `council/`, `internal/**`) and the `agents/tracing/` submodule.
- Example programs under `examples/**` are reviewed on a best-effort basis. They demonstrate SDK usage; production deployments should re-audit them for their own threat model.

Out of scope:

- Generated code under `grpc/gen/**` (regenerated from upstream proto definitions; report issues to <https://github.com/xai-org/xai-proto>).
- Behaviour or data exposure on the xAI server side; report those to xAI directly.
- Vulnerabilities that require an attacker to already have control of the host process, the API key, or the operator account.

## Threat model

The SDK assumes:

- The xAI API endpoint is trusted but not guaranteed honest. The SDK applies client-side defense in depth: bounded response reads (`Transport.MaxResponseBytes`), HTTPS-enforced base URL, capped error bodies, jittered retry backoff.
- The model is part of the trust boundary whenever the operator wires tools that drive code execution, filesystem access, or network access. The SDK ships sandboxing primitives (`agents/sandbox.DockerExecutor` with hardening defaults, `agents/harness.LocalFS` rooted at a single directory) and SSRF guards (`discord.FetchAsDataURI`); the operator is responsible for choosing them.
- User input flowing into agent or chat APIs is untrusted and should be treated accordingly by the operator (rate limiting, prompt-injection mitigation, content scoping). The SDK does not sanitise user input on the operator's behalf.
- Persistent artifacts (`agents/durable.FileStore` checkpoints, `memory.File` JSON files) are written to operator-controlled paths and read back trusting the disk contents. Do not place them on shared writable directories.

## Supported versions

Security fixes ship against `main` and the most recent tagged minor version. Older releases are supported only at the maintainer's discretion.

## Recognition

Reporters who follow this policy and act in good faith are credited in the resulting advisory unless they prefer to remain anonymous. There is no monetary bug bounty.

## See also

- [.github/workflows/ci.yml](./.github/workflows/ci.yml) for the security and quality gates run on every push and pull request.
- The [Go security policy](https://go.dev/security/policy) for reporting issues against the Go toolchain itself.
