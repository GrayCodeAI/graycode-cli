# Hawk Developer Path

This guide explains what `hawk path` checks and how to get a fresh developer machine ready to use Hawk safely.

## What "developer path" means

For Hawk, the developer path is the minimum local setup required to chat, edit code, and keep credentials off disk:

- A provider credential stored in the OS secret store
- A model selected in Hawk settings
- A local model catalog available through eyrie
- No plaintext API keys left in `provider.json` or legacy env files
- Safe defaults for Bash execution and filesystem access
- Optional but healthy ecosystem integrations like yaad memory

Run the report at any time:

```bash
hawk path
hawk path --strict
hawk doctor
hawk preflight
```

## Setup checklist

### 1. Build and workspace setup

If you are contributing from source, clone Hawk and fetch the support repos first:

```bash
git clone https://github.com/GrayCodeAI/hawk && cd hawk
make setup
go build -o hawk ./cmd/hawk
```

`make setup` populates `external/` and syncs `go.work`, which Hawk expects in contributor builds.

### 2. Configure credentials

Start Hawk and use `/config` to paste an API key or configure a local provider like Ollama.

Hawk stores credentials in the macOS Keychain or Linux secret store. It should not rely on shell env vars, `.env`, or plaintext config files for provider secrets.

Useful checks:

```bash
hawk credentials status
hawk preflight
```

### 3. Select a model

Pick a model in `/config`. Hawk stores the selected model in settings and uses eyrie for provider routing and catalog resolution.

If the catalog is missing or empty:

```bash
hawk models refresh
```

## Security checks

`hawk path` treats these as important security conditions:

- `provider.json` must not contain secret fields
- legacy `~/.hawk/env` or `~/.hawk/.env` files should be migrated away
- sensitive files like provider config and SSH paths should be blocked from agent reads

If Hawk detects old plaintext secrets, run Hawk once and complete `/config`, or remove the secret fields manually after backing up the file.

Read the full credential and isolation model in [SECURITY-DEVELOPER.md](./SECURITY-DEVELOPER.md).

## Sandbox checks

When Docker is available, Hawk prefers containerized Bash execution for stronger isolation. If Docker is unavailable, `hawk path` warns but does not block normal local development.

Use strict mode if you want Docker to be required:

```bash
hawk path --strict
```

## Ecosystem checks

`hawk path` also verifies the core support layer behind Hawk:

- `eyrie` for provider routing and preflight readiness
- `tok` for token estimation and compression
- `yaad` for optional persistent memory

If you want the broader status summary:

```bash
hawk ecosystem
hawk doctor
```

## Typical recovery path

If `hawk path` says you are not ready, this is the intended order:

1. Run `hawk`
2. Open `/config`
3. Paste an API key or configure Ollama
4. Pick a model
5. Re-run `hawk preflight`
6. Re-run `hawk path`

If security items still fail, fix those before treating the machine as ready.
