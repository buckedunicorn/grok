# OpenClaw with xAI Grok

[`openclaw/openclaw`](https://github.com/openclaw/openclaw), TypeScript personal-AI-assistant gateway with 20+ messaging channels (WhatsApp, Telegram, Slack, Discord, iMessage, etc.). Local-first; supports many model providers via its [Models config](https://docs.openclaw.ai/concepts/models).

## Setup

```sh
npm install -g openclaw@latest
openclaw onboard --install-daemon
export XAI_API_KEY=...
```

## Recipe

The most reliable path is the interactive wizard, it lets you pick a provider and validates the connection before persisting. Run:

```sh
openclaw onboard
# When prompted for the model provider, choose "OpenAI compatible".
# Endpoint: https://api.x.ai/v1
# API key:  $XAI_API_KEY
# Model:    grok-4-1-fast-reasoning
```

Or edit the models config directly. The exact path and schema vary by version; check `openclaw doctor` for the resolved location. Typical shape:

```json
{
  "models": {
    "default": {
      "provider": "openai-compatible",
      "baseURL": "https://api.x.ai/v1",
      "apiKey": "${env:XAI_API_KEY}",
      "model": "grok-4-1-fast-reasoning"
    }
  }
}
```

Then start the gateway:

```sh
openclaw gateway --port 18789 --verbose
openclaw agent --message "Ship checklist" --thinking high
```

## Verify

```sh
openclaw doctor
```

Should show the configured model, the resolved API endpoint, and surface any DM-policy or sandbox configuration issues.

## Notes

- OpenClaw's per-channel routing, sandboxing, and DM-pairing security are gateway-level features, independent of which model provider you choose.
- Auth profile rotation and model failover (multiple providers chained as fallbacks) work the same way for xAI as for any other OpenAI-compatible endpoint. You can list xAI alongside other providers and let OpenClaw fail over between them.
- For NVIDIA NemoClaw users: NemoClaw wraps OpenClaw inside a hardened sandbox, so this same model config (passed through NemoClaw's blueprint) routes Grok through the OpenShell-managed inference layer. See [NVIDIA/NemoClaw](https://github.com/NVIDIA/NemoClaw).
- OpenClaw's tool/skill system runs locally; the model just sees standard OpenAI-shaped tool calls. xAI's native function calling handles them natively.
