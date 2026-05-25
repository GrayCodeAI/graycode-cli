# Hawk developer security model

This document describes how hawk and eyrie handle API keys and agent isolation for an individual developer on macOS or Linux (no Vault, no proxy). Teams and enterprise deployment models come later.

## Goals

- API keys live only in the OS secret store (macOS Keychain / Linux GNOME Keyring or KWallet).
- Hawk does not read API keys from `.env`, shell env, or plaintext files.
- `~/.hawk/provider.json` holds routing and deployment metadata only — never secrets on disk.
- Hawk talks to eyrie without putting keys in JSON or chat messages.
- Agents run Bash inside Docker when possible; file tools cannot read credential paths.

## Credential storage

| Write | Read | Remove |
|-------|------|--------|
| `/config` paste flow → eyrie `runtime.SetCredential` | `credentials.LookupSecret` (keychain only) | `/config key remove` or `hawk credentials remove` |

On startup, hawk calls `PrepareCredentialDiscovery()` to one-time migrate legacy `~/.hawk/env` / `~/.hawk/.env` into the keychain and delete those files.

Check status: `hawk credentials status`, `hawk path`, or `hawk preflight`.

## First-run flow (`/config`)

```
User pastes API key in /config
        |
        v
hawk PersistAPIKey -> eyrie runtime.SetCredential (OS secret store)
        |
        v
eyrie Apply / discover (credentials from store, not JSON body)
        |
        v
SetupUI JSON (display_name + canonical_id per model)
        |
        v
User picks model -> settings.json (canonical id only)
```

Remove a stored key: `/config key remove` (interactive picker).

## Hawk to eyrie

- **Apply**: credentials passed from the OS store; no `api_key` fields in request payloads.
- **Chat**: `model_id` + messages only; eyrie resolves provider and reads secrets internally.

## Agent isolation

```
+------------------+     +------------------+
|  Hawk TUI/host   |     |  Docker sandbox  |
|  Keychain access |     |  Bash only       |
|  /config paste   |     |  project mount   |
+------------------+     +------------------+
         |                          |
         |  ContainerExecutor       |
         +--------------------------+
```

When the container is ready, `session.ContainerExecutor` runs Bash in the container.

### Blocked for agents (host or container policy)

- **Read** tool: `~/.hawk/env`, `~/.hawk/.env`, `~/.hawk/provider.json`, `~/.ssh/*`, etc.
- **Bash**: `printenv`, `env`, reading hawk env paths, echoing `*_API_KEY` variables.

Use `--no-container` only for debugging; secure mode warns because host Bash can access more of the filesystem.

## Migration

- **Legacy env files**: `MigrateLegacyEnvFile()` on startup imports `~/.hawk/env` / `~/.hawk/.env` → keychain → deletes files.
- **provider.json secrets**: `MigrateProviderSecrets()` strips secret fields (backup: `provider.json.pre-secret-migrate.bak`).

## Environment variables

Non-secret overrides only (hawk does not load provider API keys from env):

| Variable | Meaning |
|----------|---------|
| `HAWK_CONFIG_DIR` | Override hawk config directory |
| `OPENAI_MODEL` | Override default OpenAI model |
| `OLLAMA_BASE_URL` | Ollama server URL (also saved via `/config` for Ollama) |

## Related code

- Hawk: `internal/config/credentials_store.go`, `migrate_provider_secrets.go`, `internal/tool/safety.go`, `cmd/credentials.go`
- Eyrie: `credentials/`, `config/discovery_env.go`, `setup/setup_ui.go`
