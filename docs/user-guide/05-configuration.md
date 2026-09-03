# Configuration

Graycode reads configuration from settings files, environment variables, and has defaults for all options. This document covers the common configuration options.

---

## Precedence

Configuration is resolved in this order (highest priority first):

1. **CLI flags** (e.g., `--provider`, `--model`)
2. **Environment variables**
3. **User settings** (`~/.graycode/settings.json`)
4. **Project settings** (`.graycode/settings.json`)
5. **Built-in defaults**

---

## Settings File

Location: `~/.graycode/settings.json`

This is the main configuration file. Graycode writes to it when you save changes via `/config` or `/autonomy save`.

### Basic Settings

```json
{
  "default_provider": "openai",
  "default_model": "gpt-4o",
  "deployment_routing": true
}
```

### Autonomy Configuration

```json
{
  "autonomy": {
    "tier": "builder",
    "sandbox": "workspace",
    "dry_run": false
  }
}
```

**Tiers:**
- `always_ask` — prompts for permission on every tool call
- `scout` — classifier approves safe tools
- `builder` — broader tool access for development
- `operator` — full tool access for trusted operations
- `autonomous` — no permission prompts

**Sandbox profiles** control permissions inside Graycode's mandatory Docker
execution boundary:
- `off` — no additional policy restrictions
- `workspace` — filesystem access limited to project directory
- `strict` — minimal access, cwd only
- `devbox` — container-oriented development policy

### Agent Configuration

```json
{
  "autonomy": {
    "rules": {
      "allow": ["Bash(git:*)"],
      "deny": ["Bash(rm -rf *)"]
    }
  }
}
```

### Custom Models

Add custom model endpoints:

```json
{
  "models": {
    "my-custom-model": {
      "model": "custom-model-id",
      "base_url": "https://api.example.com/v1",
      "api_key": "sk-...",
      "name": "Display Name",
      "context_window": 128000
    }
  }
}
```

---

## Environment Variables

Key environment variables for configuration.

### Authentication

| Variable | Description |
|----------|-------------|
| `XAI_API_KEY` | xAI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GEMINI_API_KEY` | Google Gemini API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |
| `OLLAMA_BASE_URL` | Ollama endpoint (no key required) |

### Features

| Variable | Description |
|----------|-------------|
| `GRAYCODE_Y0_FOLDER_TRUST` | Folder trust feature flag (default: `1`) |
| `GRAYCODE_Y0_MARKETPLACE` | Marketplace feature flag (default: `0`) |
| `GRAYCODE_DEPLOYMENT_ROUTING` | Enable deployment-aware routing |

### Paths

| Variable | Description |
|----------|-------------|
| `GRAYCODE_HOME` | Override config directory (default: `~/.graycode`) |

---

## Project Configuration

Place configuration in `.graycode/` within your repository:

| File | Purpose |
|------|---------|
| `.graycode/settings.json` | Project settings (autonomy, rules) |
| `.graycode/sandbox.toml` | Custom sandbox profiles |
| `.graycode/lsp.json` | LSP server configuration |
| `AGENTS.md` | Project instructions |

---

## Sandbox Profiles

Location: `~/.graycode/sandbox.toml` (user) or `.graycode/sandbox.toml` (project)

Define custom sandbox profiles:

```toml
[profiles.strict]
extends = "workspace"
deny = ["**/.env", "**/*.pem", "**/credentials/**"]

[profiles.ci]
extends = "strict"
network = "deny"
```

**Built-in profiles:**
- `off` — no restrictions
- `workspace` — project directory access
- `strict` — minimal access
- `devbox` — container-oriented development policy

---

## MCP Servers

Configure MCP servers in `.graycode/settings.json` or project `.graycode/settings.json`:

```json
{
  "mcp_servers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_xxx" }
    },
    "postgres": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres", "postgresql://..."]
    }
  }
}
```

---

## Folder Trust

Folder trust controls whether project automation (hooks, plugins, MCP, LSP) can run.

### Trust Commands

```
/trust                  # Manage folder trust
/trust status           # Check trust status
/trust revoke <path>    # Revoke trust for a folder
```

### Trust Store

Location: `~/.graycode/trusted_folders.toml`

```toml
[[folders]]
path = "/Users/me/projects/my-app"
trusted_at = "2026-07-16T00:00:00Z"
```

Projects must be trusted before their hooks, plugins, MCP servers, or LSP servers can execute.

---

## Where to Go Next

| Document | What You Will Learn |
|----------|-------------------|
| [Theming](06-theming.md) | TUI appearance and themes |
| [MCP Servers](07-mcp-servers.md) | External tool integrations |
| [Skills](08-skills.md) | Installing and using skills |
| [Plugins](09-plugins.md) | Multi-component plugins and marketplace |

---

© 2026 GrayCode AI. All rights reserved.
