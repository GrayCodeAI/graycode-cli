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

11 providers registered (see `providerSpecs()`):

| Provider | ID | Credential | Strategy |
|----------|----|------------|----------|
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | remote_then_live |
| OpenAI | `openai` | `OPENAI_API_KEY` | remote_then_live |
| Google Gemini | `gemini` | `GEMINI_API_KEY` | remote_then_live |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` | live_only |
| xAI (Grok) | `grok` | `XAI_API_KEY` | remote_then_live |
| Z.AI | `z-ai` | `ZAI_API_KEY` | live_only |
| CanopyWave | `canopywave` | `CANOPYWAVE_API_KEY` | live_only |
| OpenCode Go | `opencodego` | `OPENCODEGO_API_KEY` | remote_then_live |
| Kimi (Moonshot) | `kimi` | `MOONSHOT_API_KEY` | live_only |
| Xiaomi (MiMo) | `xiaomi` | `XIAOMI_API_KEY` | live_only |
| Ollama (local) | `ollama` | `OLLAMA_BASE_URL` | live_only |

### Model strategies

- **remote_then_live**: Models come from the published remote catalog, enriched with live API data (pricing, context windows, capabilities) when credentials are present.
- **live_only**: Models come exclusively from the live provider API. Without credentials, zero models are available. The remote catalog may seed initial entries but they are replaced entirely on live fetch.

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
// Unified model listing — auto-selects cache or live
models, err := eyrieclient.ListModels(ctx, eyrieclient.ListModelsOpts{
    ProviderID: "anthropic",
    Source:     eyrieclient.ListSourceAuto, // or Cache / Live
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
5. Ensure models exist in remote catalog JSON (unless `StrategyLiveOnly`)
6. Add probe base URL for credential validation
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
