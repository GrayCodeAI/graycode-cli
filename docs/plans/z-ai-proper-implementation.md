# Z.AI Proper Gateway Implementation Plan

**Status:** Plan (ready for implementation)  
**Date:** 2026 (current)  
**Owners:** Hawk + Eyrie teams (cross-repo via go.work)  
**Related:** Xiaomi MiMo per-plan/region split (the direct precedent)  
**Goal:** First-class support for Z.AI (Zhipu/GLM) **Coding Plan** (subscription/quota, dedicated endpoint) alongside general **pay-as-you-go** API, with **region awareness** (global vs CN), matching the maturity, dynamism, reuse, reliability, and UX of the Xiaomi implementation while preserving the "live when configured + registry-driven" architecture.

---

## 1. Executive Summary

Current Z.AI support is a single generic live-only OpenAI-compat gateway (`z-ai` / `z-ai-direct`, `ZAI_API_KEY`, default `https://api.z.ai/api/paas/v4`). This is insufficient.

Z.AI reality (confirmed against official quick-start and tooling usage):
- **GLM Coding Plan**: Subscription-based (Lite/Pro/Max tiers, prompt/quota model with 5-hour rolling windows + MCP quotas). Marketed for Cursor, Claude Code, Cline, etc. **Must** use the dedicated coding endpoint for correct billing/quota consumption and plan-eligible models.
- **General API (pay-as-you-go)**: Standard token billing on the general endpoint.
- **Endpoints** (from Z.AI developer docs):
  - General: `https://api.z.ai/api/paas/v4` (or CN equivalent)
  - Coding Plan: `https://api.z.ai/api/coding/paas/v4`
- **Regions**: `api.z.ai` (global/international branding, primary for Coding Plan docs) vs China platform (`open.bigmodel.cn` / bigmodel.cn family). Affects billing, quotas, latency, and model availability. CN equivalents of the coding path exist or are expected.
- Reference catalog currently lists only minimal `z-ai/glm-4.5-air:free`; real breadth (GLM-4.5/4.7/5/Flash/V variants, vision/tooling) comes via live `/models` on the correct base.

The architecture (ProviderSpec + live fetchers + decorator clients + Hawk gateway surface) is already excellent: dynamic (new gateway = spec + fetcher + data), heavily reused (OpenAI client + compat flags + ProtocolRouter patterns), reliable (retriable-only failover, negative caching, probes), secure (centralized CredentialEnv, no secrets in JSON), and fast (compiled catalog, on-demand counts).

**The gap is only specialization surface for Z.AI**, exactly analogous to the pre-split state of Xiaomi MiMo (which received dedicated `xiaomi_mimo_token_plan` + `payg`, region picker, `catalog/xiaomi/`, dual-protocol client, Hawk UI, config resolution, and detailed docs).

This plan adds **one new setup gateway** (`z_ai_coding`) while keeping the existing `z-ai` (general) fully backward-compatible. Total setup gateways become 19. No breaking changes for existing users.

---

## 2. Current State (Precise Inventory)

### Eyrie (external/eyrie)
- `catalog/registry/providers.go:68` (single entry):
  ```go
  {
      ProviderID: "z-ai", DisplayName: "Z.AI", DeploymentID: "z-ai-direct", SortOrder: 7,
      RequiresKey: true, CredentialEnv: "ZAI_API_KEY",
      BaseURLEnv: []string{"ZAI_BASE_URL", "ZAI_API_BASE", "OPENAI_BASE_URL", "OPENAI_API_BASE"},
      ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.z.ai/api/paas/v4",
      LiveFetcherKey: "z-ai", LiveCatalogKey: "z-ai",
      APIProtocolID: "openai-chat-completions", AdapterID: "z-ai",
  },
  ```
- `catalog/live/fetchers.go:26,50,729`:
  - `DefaultZAIBaseURL = "https://api.z.ai/api/paas/v4"`
  - Registry: `"z-ai": FetchZAI`
  - `FetchZAI`: `fetchOpenAICompatModels(..., envOr(..., "ZAI_BASE_URL", DefaultZAIBaseURL), "ZAI_API_KEY", "Bearer")` + `enrichFromOpenRouter(entries, "z-ai/")`
- `setup/deployment.go:223`:
  - `"z-ai-direct"` → `client.NewOpenAIClient(..., &client.ZAICompat)`
- `client/compat.go:43`:
  - `ZAICompat = OpenAICompatConfig{ ThinkingFormat: "zai", MaxTokensField: "max_tokens", SupportsUsageInStreaming: true }`
- `config/providers.go:27`, `config/profiles.go`, `config/provider_env.go`, `config/runtime.go`, etc.: `ProviderZAI`, `ZAIRuntimeProfile`, `DefaultZAIOpenAIBaseURL`, env collection for `ZAI_API_KEY` / `ZAI_BASE_URL*`.
- `catalog/live/zai_test.go` exists (thin coverage noted in docs).
- No `catalog/zai/` subpackage (unlike `catalog/xiaomi/`).

### Hawk
- `internal/config/catalog_api.go`:
  - `AllSetupGateways()` pulls from `registry.CredentialRegistry()` (dynamic).
  - `setupGatewayRegistryID` switch already has `case "zai": return "z-ai"` (plus xiaomi special cases, google→gemini, xai→grok).
  - `GatewayDisplayName`, `IsSetupGateway`, `GatewayForModel`, `ActiveGateway` all go through the normalizer.
- `cmd/chat_config_gateways.go`: Special-case only for `ProviderXiaomiTokenPlan` (region flow before key paste, hints in footer).
- No `chat_config_zai.go` or `internal/config/zai_setup.go`.
- `internal/config/catalog_gateways_test.go:14`: Hard `len(gws) != 18` + explicit want list (includes the two xiaomi + two minimax).
- `internal/config/xiaomi_setup.go` + `cmd/chat_config_xiaomi.go` + `xiaomi_setup_test.go`: The full Hawk-side pattern for region-aware plan gateways.
- `internal/config/eyrie_apply.go` and `credentials_store.go`: Xiaomi-specific Apply/region env injection.

### Docs (stale in places)
- `external/eyrie/docs/guides/CREDENTIAL-SETUP-FLOW.md`: Lists 12 gateways (stale), has a full "Xiaomi MiMo (two gateways...)" subsection with tables for keys/bases/paths. Z.AI is one line: "live /models only".
- `external/eyrie/docs/guides/DYNAMIC-MODEL-DISCOVERY.md`: Notes "thin test coverage (z-ai...)", "All 12 setup gateways", Z.AI row describes only generic OpenAI-compat.
- Hawk `docs/DYNAMIC-MODELS.md` and others reference the gateway surface generically.
- Reference catalog (langdag) has minimal data for z-ai.

### Architecture Strengths (no changes needed)
- Everything funnels through ProviderSpec + live fetch + `runtime.ListModels(Source: auto)`.
- Decorators (Weighted/Fallback/RateLimit/Tracing/ProtocolRouter) are provider-agnostic.
- Credential centralization + guardian in Hawk front everything.
- `go work sync` + submodule hygiene enforced in CI.

---

## 3. Xiaomi Precedent (Copy This Pattern)

Xiaomi split was the first "billing plan + region + special hosts" case.

**Eyrie additions:**
- Two `ProviderSpec` rows with distinct `ProviderID`, `DisplayName`, `CredentialEnv`, `BaseURLEnv`, `LiveFetcherKey`, `LiveCatalogKey`, `DeploymentID`, `ProbeBaseURL` (empty for token plan because resolved).
- New package `catalog/xiaomi/`:
  - `endpoints.go`: `Billing`/`Region` types + constants for every host (payg + 3 token-plan regions × OpenAI + Anthropic), `NormalizeRegion`, `BillingForProvider`, `ResolveOpenAIBase`/`ResolveAnthropicBase` (override wins, region required for token plan), key-shape mismatch hints (`tp-` vs `sk-`).
  - `platform.go` + `http.go`: Separate platform catalog fetch for rich metadata (context/pricing/names) because inference `/v1/models` is sparse. `ApplyPlatformMetadata`.
- `client/mimo.go`: `NewMiMoClient` (dual OpenAI + Anthropic bases, compat, retriable failover via existing machinery).
- `config/xiaomi_profile.go`: Env consts (`EnvXiaomi*`), `ResolveXiaomiOpenAIBase`/`ResolveXiaomiAnthropicBase` (load provider.json + delegate to catalog/xiaomi), `IsXiaomiMimoProvider`, legacy migration.
- `setup/deployment.go`: `newMiMoDeploymentClient` that resolves bases via config + xiaomi package before `NewMiMoClient`.
- Registry live fetchers: `FetchXiaomiPayg` + `FetchXiaomiTokenPlan` (registered under the two keys).

**Hawk additions (thin UI + bridge only):**
- `internal/config/xiaomi_setup.go`: `ProviderXiaomiTokenPlan` const, `NeedsXiaomiTokenPlanRegion`, `SetXiaomiTokenPlanRegion` (persist + set envs for probe + derive base), `XiaomiTokenPlanRegionLabel`, `ApplyXiaomiTokenPlanRegionEnv`.
- `cmd/chat_config_xiaomi.go`: Region list (cn/sgp/ams), picker view, key handler that calls Set + invalidates cache + routes to key paste or post-save flow. Special hints.
- `cmd/chat_config_gateways.go`: In `handleConfigGatewaysSelect` and hint rendering: if the row is the token-plan gateway and needs region (or no key), launch region flow first.
- Tests + `catalog_gateways_test.go` updates.
- `eyrie_apply.go` etc. call the Apply*Env hook.

**Result:** Users see two distinct rows in /config, get region prompt only for token plan, correct hosts are used for probe/fetch/chat, key mismatch hints, rich models, full docs.

Z.AI needs the same treatment (plan split + region), but likely simpler client side (no Anthropic dual path documented yet; both paths are OpenAI-compat with the existing "zai" thinking format).

---

## 4. Proposed Design

### 4.1 Registry Entries (external/eyrie/catalog/registry/providers.go)
Add after the existing z-ai (keep the original as general payg for backward compat + users who intentionally use general API):

```go
{
    ProviderID: "z-ai", DisplayName: "Z.AI", DeploymentID: "z-ai-direct", SortOrder: 7,
    // ... (unchanged, general /paas/v4)
},
{
    ProviderID: "z_ai_coding", DisplayName: "Z.AI — Coding Plan", DeploymentID: "z_ai_coding-direct", SortOrder: 7, // or 19 after re-sort
    RequiresKey: true, CredentialEnv: "ZAI_CODING_API_KEY",
    BaseURLEnv: []string{"ZAI_CODING_BASE_URL", "ZAI_BASE_URL", "OPENAI_BASE_URL", "OPENAI_API_BASE"},
    ProbeKind:  ProbeOpenAIModels, ProbeBaseURL: "https://api.z.ai/api/coding/paas/v4",
    LiveFetcherKey: "z_ai_coding", LiveCatalogKey: "z_ai_coding",
    APIProtocolID: "openai-chat-completions", AdapterID: "z-ai",
},
```

(Alternative naming: `z_ai_coding_plan` to match `xiaomi_mimo_token_plan` verbosity. `z_ai_coding` is shorter and clear in TUI. Choose one; document alias handling.)

Add `case "z_ai_coding", "zai-coding", "z-ai_coding": return "z_ai_coding"` in Hawk's `setupGatewayRegistryID`.

### 4.2 New Eyrie Package: catalog/zai/ (modeled exactly on catalog/xiaomi/)
- `endpoints.go`:
  - Types: `Plan` ("general" | "coding"), `Region` ("global" | "cn" or more specific if needed).
  - Constants for bases:
    - General global: `https://api.z.ai/api/paas/v4`
    - Coding global: `https://api.z.ai/api/coding/paas/v4`
    - CN variants (research + docs): `https://open.bigmodel.cn/api/paas/v4`, `https://open.bigmodel.cn/api/coding/paas/v4` (or the actual CN coding host; confirm at implementation time).
  - `NormalizeRegion`, `PlanForProvider`, `ResolveOpenAIBase(plan, region, override string)`.
  - Optional: key hinting if dashboard produces distinguishable prefixes for coding keys.
- `platform.go` or enrichment (optional; start with OpenRouter "z-ai/" enrichment which FetchZAI already does; add dedicated if Z.AI coding catalog differs significantly).
- Tests: `endpoints_test.go` (table-driven, like xiaomi).

### 4.3 Live Fetchers (catalog/live/fetchers.go)
- Keep `FetchZAI` for the `z-ai` key (general).
- Add:
  ```go
  "z_ai_coding": FetchZAICoding,
  ```
- Implement `FetchZAICoding` (or a single `FetchZAIWithPlan`):
  - Resolve base via new `config.ResolveZAIOpenAIBase("z_ai_coding", cfg)` (or env first).
  - Call `fetchOpenAICompatModels(..., resolvedBase, key, "Bearer")`.
  - Same OpenRouter enrichment (or "z_ai_coding/" if they publish distinct).
- Export `DefaultZAICodingBaseURL` etc. in `config/providers.go`.

Update Registry map and any `fetchers_test` / `live_test`.

### 4.4 Client + Deployment + Config (minimal)
- `setup/deployment.go`: Add case `"z_ai_coding-direct":` → resolve base (via new config helper + LoadProviderConfig) then `NewOpenAIClient(apiKey, resolved, &client.ZAICompat)`. Reuse the same compat (thinking "zai" format applies).
- `config/providers.go`: Add `DefaultZAICodingOpenAIBaseURL`.
- `config/xai_profile.go` or new `config/zai_profile.go` (or extend existing ZAI bits):
  - Env consts: `EnvZAICodingAPIKey`, `EnvZAICodingBaseURL`, `EnvZAICodingRegion` (or plan-specific).
  - `ResolveZAIOpenAIBase(providerID string, cfg *ProviderConfig)`.
  - Migration for any legacy.
- `profiles.go` / `provider_env.go` / `runtime.go`: Wire the new provider ID into profiles, env collection, and `ZAICodingRuntimeProfile` if distinct mode needed (likely same "openai" mode).
- No new client file needed initially (reuse OpenAI path + ZAICompat). If future dual-protocol or coding-specific headers appear, add `NewZAIClient` parallel to MiMo.

### 4.5 Hawk Surface (UI + Bridge)
- `internal/config/zai_setup.go` (new, modeled 1:1 on `xiaomi_setup.go`):
  ```go
  const ProviderZAICoding = "z_ai_coding"

  func NeedsZAIRegionOrPlan(providerID string) bool { ... }
  func SetZAIRegion(...) error { ... }
  func ZAIRegionLabel() string { ... }
  func ApplyZAIRegionEnv(ctx context.Context) { ... }  // sets process envs before probe
  ```
  Delegate to `eyriecfg` (new Resolve helpers) + `catalog/zai`.
- `cmd/chat_config_zai.go` (new):
  - Region/plan options (e.g. "Global (Coding)", "China (Coding)", "Global (General)" — or separate flows).
  - View + key handler. Special footer hints: "Coding Plan keys from z.ai dashboard · uses /coding/paas/v4".
- `cmd/chat_config_gateways.go`:
  - In select + hints: if row.ID == hawkconfig.ProviderZAICoding && needs region/plan → launch zai flow (like Xiaomi).
  - Update any hardcoded Xiaomi-only hints to a helper or switch.
- Update `catalog_gateways_test.go`: change `18` → `19`, add "z_ai_coding" to want list or remove brittle explicit map.
- `eyrie_apply.go`, startup, cache invalidation: call the new Apply hook for the coding provider.
- `catalog_api.go`: add alias cases in `setupGatewayRegistryID` (keep the switch small; long-term consider adding `Aliases []string` to ProviderSpec + derive logic in eyrie registry to kill the switch).

### 4.6 Other Surfaces
- Credentials migrate/alias: `credentials/store.go` etc. for `zai_coding_api_key` → `ZAI_CODING_API_KEY`.
- Runtime profiles and deployment env sync.
- Any conformance or verify tests that enumerate providers.

---

## 5. Implementation Phases (Actionable, File-by-File)

### Phase 0 — Foundations (Eyrie, no UX yet)
1. Add the second `ProviderSpec` row in `external/eyrie/catalog/registry/providers.go`.
2. Add consts + `ResolveZAIOpenAIBase` (and region/plan types) in a new `external/eyrie/catalog/zai/endpoints.go` (copy structure from xiaomi/endpoints.go; include CN bases once confirmed).
3. Update `external/eyrie/catalog/live/fetchers.go`:
   - New default const.
   - New fetcher func + registration `"z_ai_coding": FetchZAICoding`.
   - (FetchZAI stays for the general key.)
4. `external/eyrie/config/providers.go`: new `DefaultZAICodingOpenAIBaseURL`.
5. `external/eyrie/setup/deployment.go`: add case for `z_ai_coding-direct` (resolve base first).
6. Wire minimal profile/env bits (can live in existing ZAI sections or small new `zai_profile.go` modeled on `xiaomi_profile.go`).
7. Update `external/eyrie/catalog/live/zai_test.go` (or add `zai_coding_test.go`) + any live parity tests.
8. `go test -race ./external/eyrie/catalog/...` (and full package).

**Deliverable:** `z_ai_coding` appears in `registry.All()` and can be resolved; live fetch works when `ZAI_CODING_API_KEY` + correct base is set.

### Phase 1 — Eyrie Config + Runtime Polish
- Full resolution + provider.json storage for region/plan (parallel to `XiaomiMimo*` fields).
- Legacy migration if anyone had custom ZAI_BASE_URL pointing at coding before.
- Ensure `runtime.ListModels` + discover use the right fetcher key per deployment.
- Update any default model / catalog bootstrap for the new provider ID.

### Phase 2 — Hawk UI + Config Bridge
1. Create `internal/config/zai_setup.go` + `_test.go` (table-driven; use `credentials.MapStore`).
2. Create `cmd/chat_config_zai.go` + `_test.go` (region/plan picker modeled exactly on xiaomi; include "g" hotkey support for "change region/plan").
3. Edit `cmd/chat_config_gateways.go`:
   - Import and use the new const.
   - Add conditionals for the coding provider ID in select/hints (extract a small helper if the if-chain grows).
4. Edit `internal/config/catalog_api.go` (add cases to the switch for aliases).
5. Edit `internal/config/eyrie_apply.go`, `catalog_startup.go`, ui caches etc. to call Apply hook for coding provider.
6. Update `internal/config/catalog_gateways_test.go` (19 gateways, "z_ai_coding" present).
7. `go test -race ./internal/config/... ./cmd/... -run 'Gateway|ZAI|Config'`.

### Phase 3 — Tests & Hardening
- Table-driven tests for resolution, fetch (with env overrides), region normalize.
- Integration-style via `scripts/test-config-flow.sh` or new zai flow test.
- Update hawk `catalog_startup_test.go`, `ui_cache_test` etc. that range over `AllSetupGateways()`.
- Run full `go test -race -count=1 ./...`.
- `make smoke`, `make ci` (local).

### Phase 4 — Documentation (required for "proper")
- `external/eyrie/docs/guides/CREDENTIAL-SETUP-FLOW.md`:
  - Fix header count.
  - Add full subsection for Z.AI parallel to Xiaomi (tables for general vs coding, global vs CN bases, key source, "Coding Plan keys from z.ai dashboard after subscribe", note that Coding Plan is intended for supported coding tools).
  - Official links (from research): Z.AI quick-start, devpack, platform dashboard.
- `external/eyrie/docs/guides/DYNAMIC-MODEL-DISCOVERY.md`: update "12" → "19", remove "thin coverage (z-ai)" note, add Z.AI row with plan/region details.
- Hawk `docs/DYNAMIC-MODELS.md` and `docs/ECOSYSTEM-CONFIG.md` if they enumerate.
- `external/eyrie/CHANGELOG.md` + Hawk `CHANGELOG.md` entries (conventional).
- Optional: contribute richer z-ai entries (including coding variants) to the reference catalog JSON.

### Phase 5 — Git / PR Hygiene (AGENTS.md)
- Work on feature branch only: `git checkout -b feat/z_ai_coding-plan-support`.
- Conventional commits (no co-author trailers — lefthook + history rules).
- `go fmt` / `go vet` / `golangci-lint` clean locally.
- Full `-race` + `make smoke` + `make ci` (or background) must be green before PR.
- `gh pr create --fill` (or with description referencing this plan).
- Address any required 8 status checks.
- After approval/CI: `gh pr merge --squash --delete-branch` (or admin if needed).
- Post-merge: verify `origin/main` clean, no lingering feature branches, `go work sync` clean, submodules updated, only main remote.
- (If history issues ever arise again: follow prior filter-branch + gh api protected-branch relax pattern, but avoid.)

---

## 6. Backward Compatibility & Migration
- Existing `z-ai` + `ZAI_API_KEY` + `ZAI_BASE_URL` (or env fallbacks) continue to target the general endpoint exactly as today. No change in behavior.
- Users with Coding Plan subscriptions will see a new row "Z.AI — Coding Plan" in the Gateways tab. They paste the plan key (separate env `ZAI_CODING_API_KEY` recommended so both can coexist).
- Old custom `ZAI_BASE_URL` pointing at coding path will still work for the general row (override wins); the new coding row will prefer its own env + resolved value.
- Provider.json fields for region/plan are additive.
- Live discovery for the new gateway ID works immediately after key save (same as Xiaomi).
- No impact on non-setup providers or aggregators.

---

## 7. Open Questions / Risks (Resolve During Implementation)
- Exact CN coding base URL? (Confirm on official CN docs / dashboard at implementation time; default to documented patterns.)
- Do Coding Plan keys have a distinguishable prefix (like Xiaomi `tp-`)? If yes, add `KeyMismatchHint` + append on probe errors.
- Does the coding endpoint return meaningfully different model metadata (pricing is quota-based, not token)? Fetcher may need light post-processing or skip certain enrichment.
- Is an Anthropic-compat path published for the coding plan (unlikely per current docs; if added later, extend like MiMo).
- Should we allow the same key env for both rows (with warning) or enforce distinct like Xiaomi? Distinct is cleaner for quota tracking.
- Reference catalog updates (optional follow-up).
- SortOrder: keep z-ai at 7; place coding immediately after or give it its own logical order.

---

## 8. Verification Checklist (Before PR + On Main)
- [ ] `AllSetupGateways()` returns 19 items including both z-ai variants; test passes.
- [ ] `/config` shows two distinct Z.AI rows with correct display names.
- [ ] Selecting Coding Plan (no region/plan set) triggers picker → persist → key paste flow.
- [ ] Probe + live list + chat all use `/coding/paas/v4` (or CN) when the coding gateway + region chosen.
- [ ] General `z-ai` row unaffected.
- [ ] `ZAI_CODING_API_KEY` and `ZAI_API_KEY` can both be stored.
- [ ] Region change ("g" or re-select) updates provider.json + derives correct base for probe/fetch.
- [ ] Full `go test -race -count=1 ./...` green.
- [ ] `make smoke` and local `make ci` (lint/vet/module hygiene) clean.
- [ ] Docs updated + table counts match reality.
- [ ] gh PR flow followed; 8 checks green on the PR; merged to main via gh; branches cleaned; main + origin in sync; no co-authors in new commits.

---

## 9. Appendix — Copy-Paste Starting Points

**Hawk bridge (internal/config/zai_setup.go skeleton):**
```go
package config

import (
  "context"
  "os"
  "strings"

  eyriecfg "github.com/GrayCodeAI/eyrie/config"
  "github.com/GrayCodeAI/eyrie/catalog/zai"
)

const ProviderZAICoding = "z_ai_coding"

func NeedsZAIRegionOrPlan(providerID string) bool { /* similar to Xiaomi */ }
func SetZAIRegionOrPlan(...) error { /* persist to provider.json via eyriecfg, set envs, derive base */ }
func ApplyZAIRegionEnv(ctx context.Context) { /* ... */ }
```

**Eyrie endpoints (external/eyrie/catalog/zai/endpoints.go):**
Copy the structure of `xiaomi/endpoints.go` (Billing/Region → Plan/Region, all the Resolve* funcs, const bases for coding/general × global/cn).

**Gateway select special case (cmd/chat_config_gateways.go):**
Add parallel to the existing XiaomiTokenPlan block (search for `ProviderXiaomiTokenPlan`).

**Test count bump:**
Only the one `len(gws) != 18` assertion + the want map in `internal/config/catalog_gateways_test.go`.

---

**End of Plan**

This document is the single source for the implementation. After writing code, update this file with "Implemented" status + links to the merged PR(s).

Follow AGENTS.md at every step: tests beside source, table-driven where multi-case, conventional signed commits, feature branch + gh PR only, full `-race` + make ci green, no direct main, ecosystem (go.work + external/eyrie) hygiene.

When ready to execute: create the feature branch and begin Phase 0 in eyrie (the registry + fetcher + catalog/zai package changes are the highest-leverage first commits).
