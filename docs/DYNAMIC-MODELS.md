# Dynamic Model Discovery — Hawk Developer Guide

## Overview

Hawk uses **eyrie** for all model discovery, provider routing, and credential management. Hawk never calls an LLM API directly or hardcodes provider logic.

The model pipeline has three layers:

```
Remote catalog (langdag.com/.../catalog.json)
    ↓ fetch
Local cache (~/.eyrie/model_catalog.json)
    ↓ merge with live API data
Compiled catalog (in-memory CompiledCatalogV1)
    ↓ query
Model picker (eyrieclient.ListModels)
```

## Provider registry

Single source of truth: `eyrie/catalog/registry/providers.go`

12 setup gateways registered (see `providerSpecs()` in eyrie):

| Provider | ID | Credential |
|----------|----|------------|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` |
| OpenAI | `openai` | `OPENAI_API_KEY` |
| Google Gemini | `gemini` | `GEMINI_API_KEY` |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` |
| xAI (Grok) | `grok` | `XAI_API_KEY` |
| Z.AI | `z-ai` | `ZAI_API_KEY` |
| CanopyWave | `canopywave` | `CANOPYWAVE_API_KEY` |
| OpenCode Go | `opencodego` | `OPENCODEGO_API_KEY` |
| Kimi (Moonshot) | `kimi` | `MOONSHOT_API_KEY` |
| Xiaomi (MiMo) Pay-as-you-go | `xiaomi_mimo_payg` | `XIAOMI_MIMO_PAYG_API_KEY` |
| Xiaomi (MiMo) Token Plan | `xiaomi_mimo_token_plan` | `XIAOMI_MIMO_TOKEN_PLAN_API_KEY` |
| Ollama (local) | `ollama` | `OLLAMA_BASE_URL` |

### Xiaomi MiMo

Two `/config` gateway rows (not one). Each product uses a **single key** for both OpenAI-compat (`/v1/chat/completions`) and Anthropic-compat (`/v1/messages`) on the matching host. Token Plan requires region `cn`, `sgp`, or `ams` before paste. Legacy `XIAOMI_MIMO_API_KEY` maps to pay-as-you-go. Details: `eyrie/docs/guides/CREDENTIAL-SETUP-FLOW.md` (MiMo section).

### Model discovery (all gateways)

Every setup gateway lists models from the provider’s live API (or Ollama tags) only. Without credentials (or Ollama URL), zero models. After paste/save, `/config` probes and `ListModels` hits the live fetcher only. There is no remote-catalog bootstrap for picker models.

## Hawk API (via `internal/eyrieclient`)

Hawk production code must use `internal/eyrieclient/` — never import eyrie packages directly.

### Catalog functions

```go
// Load the compiled catalog from cache
compiled, err := eyrieclient.LoadCompiledCatalogV1(ctx, opts)

// Query models
models := eyrieclient.ModelEntriesForProvider(compiled, "anthropic")
modelID := eyrieclient.FirstModelForProvider(compiled, "anthropic")
gateway := eyrieclient.GatewayForModel(compiled, "claude-sonnet-4-6")
providers := eyrieclient.ProviderIDsFromCompiled(compiled)
liveOnly := eyrieclient.IsLiveOnlyProvider("ollama")

// Display
label := eyrieclient.DisplayModelLabel(id, displayName)
owner := eyrieclient.DisplayModelOwner(owner, id)
```

### Credential functions

```go
name := eyrieclient.PlatformSecretStoreName()
hasKey := eyrieclient.HasSecret(ctx, "OPENAI_API_KEY")
secret := eyrieclient.LookupSecret(ctx, "OPENAI_API_KEY")
report := eyrieclient.StorageReportFor(ctx)
```

### Model listing

```go
// Unified model listing — live API for setup providers
models, err := eyrieclient.ListModels(ctx, eyrieclient.ListModelsOpts{
    ProviderID: "anthropic",
    Source:     eyrieclient.ListSourceAuto, // live for registry providers
})

// Shortcut for single provider
models, err := eyrieclient.ListModelsForProvider(ctx, "anthropic")
```

### Session construction

```go
// Build a chat client with deployment routing support
sess := eyrieclient.NewHawkSession(ctx, useDeploymentRouting, provider, model, systemPrompt, registry)
```

## Adding a new provider

1. Add one `ProviderSpec` row in `eyrie/catalog/registry/providers.go`
2. Add `CredentialsEnvFallbacks` if the API key has known alternative env var names
3. If live list API exists: implement fetcher in `catalog/live/fetchers.go` and register in `Registry` map
4. Add live fetcher key to the provider spec (`LiveFetcherKey`)
5. Add probe base URL for credential validation
7. No hawk code changes needed

## Testing model discovery

```bash
# Refresh catalog and list models for a provider
EYRIE_MODEL_CATALOG_REFRESH=1 hawk config model list --provider anthropic

# Preflight diagnostics
hawk preflight

# Doctor check
hawk doctor
```
