# Dynamic Model Discovery — Hawk Developer Guide

## Ownership and flow

Hawk is the product face; Eyrie is the provider engine. Hawk owns the CLI/TUI,
model-picker presentation, user intent, and output compatibility. The
`eyrie/engine` facade alone owns provider registry details, credentials,
discovery, catalog/cache policy, model aliases, deployment routing, and chat
transport.

```text
provider APIs / remote catalog / local cache
                     |
                     v
            eyrie/engine.Engine
          catalog + credentials + routing
                     |
          stable host DTOs and methods
                     v
       Hawk composition and presentation
          /config | models | conversation
```

Production Hawk packages must not import Eyrie packages below
`github.com/GrayCodeAI/eyrie/engine`. Shell and AST guards enforce a
zero-exception boundary.

## Hawk composition boundary

`internal/config` is Hawk's control-plane composition root. It creates an
Eyrie engine and projects engine models into Hawk UI and command contracts:

```go
models, err := config.ListEngineModels(ctx, "anthropic", false)
live, err := config.ListEngineModels(ctx, "anthropic", true)
public, err := config.ListPublicEngineModels(ctx, "xiaomi_mimo_payg")
```

Conversation construction goes through Hawk's `internal/engine` adapter. The
adapter translates Hawk-owned message, tool, usage, and stream DTOs to the
Eyrie engine facade; conversation history, WAL, resume, approvals, and tool
execution remain Hawk-owned.

## Catalog and live discovery

The Eyrie engine combines its provider registry, provider-scoped live
discovery, and the cache at `~/.eyrie/model_catalog.json` by default. Override
the cache with `EYRIE_MODEL_CATALOG_PATH`. Credential and state dependencies
are injected into each Engine instance, so discovery does not need hidden
process-global credentials.

Use the normal cache-backed path for repeatable UI and automation:

```bash
hawk models list anthropic
hawk models list anthropic --json
```

Use a provider-scoped live request when current connectivity and credentials
must be checked:

```bash
hawk models list anthropic --live
hawk models list anthropic --live --json
hawk models list anthropic --live --raw
```

`hawk preflight` reports **local readiness**: usable local state, a selected
model, and presence of the required stored credential. It is intentionally
cheap and does not prove that a remote provider accepts that credential.
Treat a successful provider-scoped `--live` request (or `/config` live
validation) as **live verified**.

## Stable command output

`hawk models list --json` is a Hawk-owned compatibility contract, not a direct
serialization of Eyrie's evolving `engine.Model` DTO. Its stable fields are:

```text
id, input_price_per_1m, output_price_per_1m, context_window, max_output,
server_tools, display_name, description, owner, live_metadata
```

New fields must be additive. `--raw` returns provider-native
`live_metadata` objects when available; for cache/public rows without native
metadata, it returns the stable Hawk compatibility row instead of `null`.

## Custom gateways

Hawk converts effective `custom_providers` settings into
`engine.Options.CustomGateways` at its composition root. Custom gateway
metadata is snapshotted per Engine instance. Do not register custom gateways
in Eyrie process-global state: tests, parallel sessions, and future multi-tenant
hosts must be isolated from one another.

## Adding or changing a provider

1. Implement registry, discovery, credentials, aliases, and transport behavior
   behind Eyrie's engine facade.
2. Add Eyrie tests for cache and live discovery, credential status, selection,
   and generation/streaming.
3. Commit and verify standalone Eyrie.
4. Advance Hawk's `../eyrie` sibling checkout to that exact commit, then update
   Hawk's module version when the Eyrie revision is published.
5. Verify both the workspace (`go.work`) and published-module
   (`GOWORK=off`) build modes.

Hawk changes are needed only for a new product behavior or an additive
Hawk-owned presentation field—not for provider-specific mechanics.

## Checks

```bash
hawk models refresh
hawk models status
hawk preflight
make eyrie-engine-guard
go test ./cmd ./internal/config ./internal/engine -count=1
```
