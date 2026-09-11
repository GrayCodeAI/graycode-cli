# Authentication

Hawk supports several authentication methods, including API key configuration through the TUI and multi-provider support via Eyrie.

---

## API Key Configuration

On first launch, Hawk opens the TUI where you can configure credentials. Press `/config` or `/autonomy` to open the configuration picker. Your API keys are stored in your OS keychain (macOS Keychain or Linux keyring), never in plain text or environment variables.

Hawk supports multiple providers:

| Provider | ID | Key |
|----------|----|-----|
| xAI Grok | `grok` | `XAI_API_KEY` |
| Anthropic Claude | `anthropic` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | `OPENAI_API_KEY` |
| Google Gemini | `gemini` | `GEMINI_API_KEY` |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` |
| Fireworks AI | `fireworks` | `FIREWORKS_API_KEY` |
| Ollama (local) | `ollama` | `OLLAMA_BASE_URL` (no API key) |

### Setting Up Credentials

```bash
# Verify credential status
hawk credentials status

# Run the TUI and use /config to set keys interactively
hawk
```

### Environment Variables (Fallback)

For CI/CD or headless environments, you can set API keys as environment variables. These serve as fallback when no keychain entry exists:

```bash
export XAI_API_KEY="xai-..."
hawk
```

Fireworks uses its OpenAI-compatible API. Set `FIREWORKS_API_KEY`; the default
base URL is `https://api.fireworks.ai/inference/v1`. See the official
[quickstart](https://docs.fireworks.ai/getting-started/quickstart),
[API reference](https://docs.fireworks.ai/api-reference/introduction), and
[models overview](https://docs.fireworks.ai/models/overview).

---

## Provider Configuration

Hawk uses Eyrie for provider routing, health checks, and retry logic. To configure providers:

```bash
# In the TUI, press /config to open provider settings
hawk

# Or validate readiness
hawk path
```

### Deployment-Aware Routing

For deployment-aware routing, set in `.hawk/settings.json`:

```json
{
  "deployment_routing": true
}
```

Or export:

```bash
export HAWK_DEPLOYMENT_ROUTING=true
```

Hawk will route canonical model IDs through Eyrie's deployment catalog. Refresh the catalog with:

```
/refresh-model-catalog
```

---

## OIDC (Customer SSO)

Authenticate developers through your own Identity Provider (IdP) — such as Okta, Azure AD, or Auth0 — instead of a single vendor.

### Configure via Settings

```json
// .hawk/settings.json
{
  "oidc": {
    "issuer": "https://acme.okta.com",
    "client_id": "0oa1b2c3d4e5f6g7h8i9"
  }
}
```

Hawk discovers endpoints via `{issuer}/.well-known/openid-configuration`, opens the IdP login page, and stores tokens in the keychain. Tokens auto-refresh silently via the stored `refresh_token`.

### Required Scopes

- `openid`
- `profile`
- `email`
- `offline_access` (enables silent token refresh)

---

## External Auth Provider

When browser-based login isn't possible — for example, on sandboxed VMs, CI runners, or air-gapped networks — delegate authentication to an external binary or script.

### How It Works

1. Hawk runs your command via `sh -c "<command>"`
2. Your binary runs whatever auth flow it needs
3. **stdout** is captured and parsed as an access token
4. **stderr** carries human-readable output surfaced to the user
5. Exit 0 = success; exit non-zero = falls back to TUI config

### Token Format

Bare string (just the raw token):

```
eyJhbGciOiJSUzI1NiIs...
```

JSON with optional refresh token:

```json
{"access_token": "eyJhbGciOi...", "refresh_token": "ref-shrike", "expires_in": 3600}
```

### Configuration

```json
// .hawk/settings.json
{
  "auth_provider_command": "/usr/local/bin/my-auth-provider",
  "auth_provider_label": "Acme Corp"
}
```

Or via environment variables:

```bash
export HAWK_AUTH_PROVIDER_COMMAND="/usr/local/bin/my-auth-provider"
export HAWK_AUTH_PROVIDER_LABEL="Acme Corp"
```

### Token Refresh

When Hawk needs to refresh an expired token, it re-runs your binary with `HAWK_AUTH_EXPIRED=1` set in the environment:

```bash
#!/bin/sh
if [ "$HAWK_AUTH_EXPIRED" = "1" ]; then
    echo "Refreshing token..." >&2
    TOKEN=$(my-company-auth --refresh --silent)
else
    echo "Authenticating via Acme Corp SSO..." >&2
    TOKEN=$(my-company-auth --login --interactive)
fi

if [ -z "$TOKEN" ]; then
    echo "Authentication failed" >&2
    exit 1
fi

echo "{\"access_token\": \"$TOKEN\", \"expires_in\": 3600}"
```

---

## Credential Status

Check credential status at any time:

```bash
hawk credentials status
```

This verifies keychain entries and validates Eyrie's provider status.

---

## Credential Precedence

Hawk resolves credentials in this order:

1. **Per-model configuration** — set via `/config` or settings
2. **Keychain entry** — obtained through TUI configuration
3. **Environment variable** — fallback (e.g., `XAI_API_KEY`)

During a session, the active method handles all refreshes.

---

## Multi-Provider Support

Hawk works with any LLM provider through Eyrie's adapter system:

| Provider | Status |
|----------|--------|
| Anthropic | Full support |
| OpenAI | Full support |
| Google Gemini | Full support |
| DeepSeek | Full support |
| Ollama (local) | Full support |
| OpenRouter | Full support |

Any provider can be set as default in your settings:

```json
{
  "default_provider": "openai",
  "default_model": "gpt-4o"
}
```

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Keyboard Shortcuts](03-keyboard-shortcuts.md) | Key bindings for the TUI |
| [Slash Commands](04-slash-commands.md) | Available `/` commands |
| [Configuration](05-configuration.md) | Settings and sandbox profiles |

---

© 2026 GrayCode AI. All rights reserved.
