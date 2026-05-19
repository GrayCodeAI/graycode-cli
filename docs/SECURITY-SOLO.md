# Hawk solo security model

This document describes how hawk and eyrie handle API keys and agent isolation for a single developer on macOS (no Vault, no proxy).

## Goals

- API keys live in the OS keychain (or legacy `~/.hawk/env` when opted out).
- `~/.hawk/provider.json` holds routing and deployment metadata only — never secrets on disk.
- Hawk talks to eyrie without putting keys in JSON or chat messages.
- Agents run Bash inside Docker when possible; file tools cannot read credential files.

## Credential storage

| Mode | `HAWK_SECURE_CREDENTIALS` | Write path | Read path |
|------|----------------------------|------------|-----------|
| Secure (default) | unset or `1` | macOS Keychain via eyrie | Keychain, then env file for migration |
| Legacy | `0` | Keychain + mirror to `~/.hawk/env` | Same |

On startup, hawk calls `PrepareCredentialDiscovery()` so eyrie discovery sees keys from keychain and env without logging values.

## First-run flow (`/config`)

```
User pastes API key in /config
        |
        v
hawk PersistAPIKey -> eyrie runtime.SetCredential (keychain)
        |
        v
eyrie Apply / discover (credentials from env, not JSON body)
        |
        v
SetupUI JSON (display_name + canonical_id per model)
        |
        v
User picks model -> settings.json (canonical id only)
```

## Hawk to eyrie

- **Apply**: process env populated from keychain; no `api_key` fields in request payloads.
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

On first run after upgrade, `MigrateProviderSecrets()` strips secret fields from existing `provider.json` (backup: `provider.json.pre-secret-migrate.bak`).

## Environment variables

| Variable | Meaning |
|----------|---------|
| `HAWK_SECURE_CREDENTIALS` | `0` disables keychain-only disk policy (allows env file mirroring) |
| Provider keys | Standard names (`OPENAI_API_KEY`, etc.) set in process during discovery only |

## Related code

- Hawk: `internal/config/credentials_store.go`, `migrate_provider_secrets.go`, `internal/tool/safety.go`
- Eyrie: `credentials/`, `config/deployment_secrets.go`, `setup/setup_ui.go`
