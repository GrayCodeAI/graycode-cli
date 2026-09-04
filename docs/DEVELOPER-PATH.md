# Graycode Developer Path

This guide explains what `graycode path` checks and how to get a fresh developer machine ready to use Graycode safely.

## What "developer path" means

For Graycode, the developer path is the minimum local setup required to chat, edit code, and keep credentials off disk:

- A provider credential stored in the OS secret store
- A model selected in Graycode settings
- A local model catalog available through eyrie
- No plaintext API keys left in Eyrie's configured `provider.json` or legacy env files
- Safe defaults for Bash execution and filesystem access
- Optional but healthy ecosystem integrations like harrier memory

Run the report at any time:

```bash
graycode path
graycode path --strict
graycode doctor
graycode preflight
```

## Setup checklist

### 1. Build and workspace setup

If you are contributing from source, clone Graycode as the main CLI. The support
repositories are independent sibling checkouts when you need the full local
workspace; they are not nested under Graycode:

```bash
mkdir graycode-eco && cd graycode-eco
git clone https://github.com/GrayCodeAI/graycode-cli
git clone https://github.com/GrayCodeAI/eyrie
cd graycode-cli
make setup
go build -o graycode ./cmd/graycode
```

`make setup` validates the canonical 15-repository manifest and regenerates the
parent `../go.work` from the nine local Go repositories. Graycode can also be built
as a standalone checkout with `GOWORK=off go build ./cmd/graycode`; the sibling
workspace is only required for cross-repository development and boundary checks.

### 2. Configure credentials

Start Graycode and use `/config` to paste an API key or configure a local provider like Ollama.

Graycode stores credentials in the macOS Keychain or Linux secret store. It should not rely on shell env vars, `.env`, or plaintext config files for provider secrets.

Useful checks:

```bash
graycode credentials status
graycode preflight
```

`graycode preflight` is a local-ready check; it does not contact a provider. To
live-verify the selected provider credential and connectivity, use `/config`
validation or `graycode models list <provider> --live`.

### 3. Select a model

Pick a model in `/config`. Graycode stores the selected model in settings and uses eyrie for provider routing and catalog resolution.

If the catalog is missing or empty:

```bash
graycode models refresh
```

## Security checks

`graycode path` treats these as important security conditions:

- Eyrie's resolved `provider.json` must not contain secret fields
- legacy `~/.graycode/env` or `~/.graycode/.env` files should be migrated away
- sensitive files like provider config and SSH paths should be blocked from agent reads

Eyrie resolves provider state from `EYRIE_CONFIG_DIR` first, then
`GRAYCODE_CONFIG_DIR` for compatibility, then the platform user-config directory.
Graycode protects that resolved path even when it is customized or symlinked.

If Graycode detects old plaintext secrets, run Graycode once and complete `/config`, or remove the secret fields manually after backing up the file.

Read the full credential and isolation model in [SECURITY-DEVELOPER.md](./SECURITY-DEVELOPER.md).

## Sandbox checks

Docker is mandatory for agent command execution. If Docker is unavailable,
`graycode path` reports a blocking failure and agent tools remain locked. Graycode does
not offer a host-execution fallback.

The versioned `graycodeai/graycode-sandbox` image is pulled automatically when it
is not already local. If the public registry is unavailable, Graycode builds its
bundled sandbox Dockerfile locally through Docker.

```bash
graycode path --strict
```

## Ecosystem checks

`graycode path` also verifies the core support layer behind Graycode:

- `eyrie` for provider routing and local preflight readiness
- `shrike` for token estimation and compression
- `harrier` for optional persistent memory

If you want the broader status summary:

```bash
graycode ecosystem
graycode doctor
```

## Typical recovery path

If `graycode path` says you are not ready, this is the intended order:

1. Run `graycode`
2. Open `/config`
3. Paste an API key or configure Ollama
4. Pick a model
5. Re-run `graycode preflight`
6. Re-run `graycode path`

If security items still fail, fix those before treating the machine as ready.
