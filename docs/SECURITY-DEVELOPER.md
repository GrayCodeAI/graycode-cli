# Graycode developer security model

This document describes how graycode and graycode-router handle API keys and agent isolation for an individual developer on macOS or Linux (no Vault, no proxy). Teams and enterprise deployment models come later.

## Goals

- API keys live only in the OS secret store (macOS Keychain / Linux GNOME Keyring or KWallet).
- Graycode does not read API keys from `.env`, shell env, or plaintext files.
- GraycodeRouter's `provider.json` holds routing and deployment metadata only — never secrets on disk.
- Graycode talks to graycode-router without putting keys in JSON or chat messages.
- Agent commands run inside mandatory Docker isolation; file tools cannot read
  credential paths.

## Credential storage

| Write | Read | Remove |
|-------|------|--------|
| `/config` paste flow → `graycode-router/engine.Engine.SaveCredential` | `Engine.ResolveCredential` (secret store only) | `/config key remove` or `graycode credentials remove` |

On startup, Graycode asks the GraycodeRouter engine facade to migrate legacy
`~/.graycode/env` / `~/.graycode/.env` values into the secret store and delete those
files. It also imports recognized historical secret fields from
`provider.json` before atomically rewriting that file with metadata only. A
secret-store or state-write failure aborts the rewrite and rolls back newly
imported values.

Check status: `graycode credentials status`, `graycode path`, or `graycode preflight`.

## First-run flow (`/config`)

```
User pastes API key in /config
        |
        v
Graycode /config -> GraycodeRouter engine credential service (OS secret store)
        |
        v
GraycodeRouter engine discover/apply (credentials from store, not JSON body)
        |
        v
SetupUI JSON (display_name + canonical_id per model)
        |
        v
User picks model -> settings.json (canonical id only)
```

Remove a stored key: `/config key remove` (interactive picker).

## Graycode to GraycodeRouter

- **Control plane**: Graycode calls only `graycode-router/engine`; no lower GraycodeRouter package is a
  production import.
- **Discovery/apply**: credentials are resolved from the Engine's injected
  secret store; provider state and request bodies remain sanitized.
- **Chat**: Graycode sends model intent, messages, and tool definitions; GraycodeRouter
  resolves the gateway and reads secrets internally.

## Agent isolation

```
+------------------+     +------------------+
|  Graycode TUI/host   |     |  Docker sandbox  |
|  Keychain access |     |  Commands only   |
|  /config paste   |     |  project mount   |
+------------------+     +------------------+
         |                          |
         |  ContainerExecutor       |
         +--------------------------+
```

When the container is ready, `session.ContainerExecutor` runs agent commands in
Docker. Graycode fails closed when Docker is unavailable; it never falls back to
host command execution.

The sandbox image has an independent compatibility version embedded in Graycode.
Startup first checks the local Docker image cache, then anonymously pulls the
public `graycodeai/graycode-sandbox` image. If the registry cannot be reached, Graycode
builds the same bundled sandbox Dockerfile locally. Registry login is not
required for users, and neither provisioning path enables host execution.

### Blocked for agents

- **Read** tool: legacy Graycode env files, GraycodeRouter's configured `provider.json`,
  `~/.ssh/*`, etc.
- **Bash**: `printenv`, `env`, reading graycode env paths, echoing `*_API_KEY` variables.

## Migration

- **Legacy env files**: startup migration imports `~/.graycode/env` and
  `~/.graycode/.env` into the OS secret store, then deletes the plaintext files.
- **provider.json secrets**: GraycodeRouter transactionally imports recognized top-level
  and deployment credentials, atomically writes sanitized metadata, and uses a
  temporary `provider.json.pre-secret-migrate.bak` only during the transaction.
- **All subsequent writes**: the GraycodeRouter engine applies the same sanitization and
  atomic-write path, so migrated secret fields cannot be reintroduced.

## Provider state path

GraycodeRouter owns the provider-state path. Resolution order is:

1. `GRAYCODE_ROUTER_CONFIG_DIR/provider.json`
2. `GRAYCODE_CONFIG_DIR/provider.json` (compatibility fallback)
3. the platform user-config directory under `graycode/provider.json`

Graycode's Read/Edit/Write and Bash safety checks protect the resolved path,
including a custom or symlinked `GRAYCODE_ROUTER_CONFIG_DIR`; protection is not limited
to the historical default provider-state location.

## Environment variables

Non-secret overrides only (graycode does not load provider API keys from env):

| Variable | Meaning |
|----------|---------|
| `GRAYCODE_CONFIG_DIR` | Override graycode config directory |
| `GRAYCODE_ROUTER_CONFIG_DIR` | Override GraycodeRouter provider-state directory; takes precedence for `provider.json` |
| `OPENAI_MODEL` | Override default OpenAI model |
| `OLLAMA_BASE_URL` | Ollama server URL (also saved via `/config` for Ollama) |

## Related code

- Graycode: `internal/config/graycode-router_engine.go`, `internal/tool/safety.go`,
  `internal/storage/paths.go`, `cmd/credentials.go`
- GraycodeRouter public host boundary: `engine/`
- Daemon HTTP surface: [`docs/DAEMON-PORT-THREAT-MODEL.md`](DAEMON-PORT-THREAT-MODEL.md)
