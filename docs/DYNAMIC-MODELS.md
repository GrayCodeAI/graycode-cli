# Dynamic models (eyrie-owned catalog + selection)

Hawk does **not** ship a hardcoded model list and does **not** store model/provider in `~/.hawk/settings.json`.

| Data | Location |
|------|----------|
| Model catalog (IDs, names, pricing) | Eyrie `~/.eyrie/model_catalog.json` |
| Selected model & provider | Eyrie `~/.hawk/provider.json` (`active_model`, `anthropic_model`, …) |
| API keys | Eyrie keychain + env |
| Hawk host prefs (theme, sandbox, tools) | `~/.hawk/settings.json` |

## Add a new model

1. Update the eyrie catalog source (bootstrap JSON, remote discover, or provider API enrichment).
2. Run catalog refresh (`hawk models refresh`, `/config` → refresh, or restart hawk with keys set).
3. Hawk shows the new model automatically — no hawk code changes.

## Change the active model

- `/config` → pick model, or `/model <id>`, or `hawk config set model <id>`
- All of these call `runtime.SetActiveModel` → `provider.json`

Legacy `model` / `provider` keys in `settings.json` are migrated into `provider.json` on first load and removed from hawk settings on save.

## Hawk integration surface

- TUI and commands call `internal/eyrieclient` → `github.com/GrayCodeAI/eyrie/runtime`.
- Do **not** import `eyrie/catalog` or `eyrie/setup` from `cmd/` except via `eyrieclient`.
- `internal/config.ActiveModel` / `SetActiveModel` delegate to eyrie runtime.

## Eyrie APIs

| API | Purpose |
|-----|---------|
| `catalog.ModelEntriesForProvider(compiled, provider)` | Filter compiled catalog |
| `runtime.ModelsForProvider(ctx, provider)` | Load cache + auto-discover if empty |
| `runtime.ActiveModel` / `SetActiveModel` | Read/write user selection |
| `runtime.Discover(ctx)` | Refresh from API keys |
| `setup.BuildSetupUI` | Provider/model groups for UI |
