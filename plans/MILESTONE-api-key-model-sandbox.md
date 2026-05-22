# Milestone: API key → model → sandbox

**Status:** credential + sandbox work complete locally; manual fresh-macOS E2E + CI push pending  
**Branch (both repos):** `feature/secure-credentials-sandbox`  
**Out of scope:** conversation DAG (`/fork`, `convo.db` as source of truth)  
**Reference layout:** sibling repos (hawk + eyrie)

| Repo | Branch | Local commit |
|------|--------|--------------|
| hawk | `feature/secure-credentials-sandbox` | `973671c` + follow-up credential cleanup |
| eyrie | `feature/secure-credentials-sandbox` | `2657c72` (includes `eac730b` Bedrock routing) |

`eyrie/main` is reset to `origin/main`; all WIP is on the feature branch only.

## Goal

A new user can:

1. Paste an API key securely (OS secret store only — macOS Keychain / Linux keyring; not `provider.json`, not `.env`)
2. Pick a model from eyrie discover output
3. Chat with tools running in Docker by default
4. Remove a stored key via `/config key remove` or `hawk credentials remove`

## Architecture

```
User /config
    → PersistAPIKey (eyrie keychain via runtime.SetCredential)
    → ApplyEyrieCredentials (discover + provider.json routing only)
    → model picker (SetupUI canonical ids)
    → settings.json (model id only)

User /config key remove
    → RemoveStoredCredential (keychain delete via picker)

hawk chat
    → PrepareCredentialDiscovery (one-time migrate ~/.hawk/env → keychain, delete files)
    → EvaluateSetup (block chat if key/model missing)
    → container boot (Docker)
    → session.StreamChat via eyrie client (keys on host keychain only)

Credential discovery (eyrie-owned, no hawk hardcoded env lists):
    catalog cache → BootstrapCatalogV1 → legacy profiles (last resort)
    → DiscoveryCredentials(ctx) from OS store only (not process env)
    → HasAnyConfiguredDeployment
```

## Phases

### Phase 0 — Plan & tracking (this doc)

- [x] Write milestone plan
- [x] Keep an **Iteration log** at the bottom updated each PR/session

### Phase 1 — API keys (secure first-run)

| # | Task | Status |
|---|------|--------|
| 1.1 | `setup_status.go`: `EvaluateSetup`, `HasConfiguredDeployment`, `NeedsFirstRunSetup` | done |
| 1.2 | Onboarding uses `PersistAPIKey` (keychain only) | done |
| 1.3 | Welcome banner shows setup CTA when keys/model missing | done |
| 1.4 | TUI auto-opens `/config` hub on first run when setup needed | done |
| 1.5 | `MigrateProviderSecrets` on every hawk start (already in root) | done |
| 1.6 | Tests: `HasConfiguredDeployment`, placeholder rejection | done |
| 1.7 | No secrets in `provider.json` on disk | done (`TestVerify_*` in `milestone_verify_test.go`) |
| 1.8 | Keychain-only: no `~/.hawk/env` writes, no `.env` credential load | done |
| 1.9 | Legacy `~/.hawk/env` / `.env` one-time migration → keychain → delete | done (`MigrateLegacyEnvFile`) |
| 1.10 | `hawk credentials status` / `remove` CLI | done |
| 1.11 | `/config key remove` (picker only; no inline provider arg) | done |
| 1.12 | Remove deprecated APIs (`DiscoveryCredentialsFromOS`, `LoadDotEnv`, `ApplyToProcess`, …) | done |

### Phase 2 — Model selection

| # | Task | Status |
|---|------|--------|
| 2.1 | After key: guided model picker (`configGuideAfterKey`) | done |
| 2.2 | Block chat send when no model (clear error → `/config`) | done |
| 2.3 | Catalog prefetch at startup when keys present | done |
| 2.4 | Friendly error when catalog empty (no keys / network) | done (`CatalogEmptyHint`, model picker + startup messages) |
| 2.5 | Setup flow: key + model clears `NeedsSetup` | done (`TestVerify_EvaluateSetupFlow`) |
| 2.6 | Stale-while-revalidate model cache + atomic catalog writes | done |

### Phase 3 — Sandbox

| # | Task | Status |
|---|------|--------|
| 3.1 | Container default on (`shouldUseContainer`) | done |
| 3.2 | Block input when container required but Docker down | done |
| 3.3 | `ContainerExecutor` wired for bash | done |
| 3.4 | Read tool blocks credential paths (`safety.go`) | done |
| 3.5 | Document `--no-container` vs secure mode | done (`SECURITY-SOLO.md`) |
| 3.6 | Container cannot read host `~/.hawk/env` | done (`isolation_verify_test.go`; skips if Docker down) + `TestIsSensitivePath` |
| 3.7 | Clarify `/sandbox` vs default container in help | done (help + flag descriptions) |

### Phase 4 — Hardening & ship

| # | Task | Status |
|---|------|--------|
| 4.1 | Commit hawk `feature/secure-credentials-sandbox` | done (`973671c`) |
| 4.2 | Commit matching eyrie credential/catalog changes | done (`2657c72` on same branch) |
| 4.3 | CI green on both repos | partial (local `go test ./...` pass; GitHub CI not run here) |
| 4.4 | Update `AGENTS.md` milestone section (not DAG) | done |
| 4.5 | Update `SECURITY-SOLO.md`, contextual help, diagnostics for keychain-only | done |
| 4.6 | `hawk preflight` + doctor credential storage section | done |

## Definition of done

- [ ] Fresh macOS: `hawk` → config opens → key → model → message works (**manual** — not run in CI agent)
- [x] `provider.json` has no API keys on disk (automated: `TestVerify_ProviderJSONOnDiskHasNoSecrets`, migrate test)
- [x] Credential files blocked from read tool (`TestIsSensitivePath` in `safety_test.go`)
- [x] API keys stored in OS secret store only (no plaintext `~/.hawk/env` after migration)
- [x] Remove key path: `/config key remove` + `hawk credentials remove`
- [ ] Docker running: bash in container end-to-end chat (**manual**; automated test skips when Docker unavailable)
- [x] DAG unchanged (optional `/fork` still best-effort only)

## Verification

Run locally:

```bash
./scripts/verify-milestone.sh
go test ./...          # hawk + eyrie
hawk credentials status
hawk preflight
```

| Check | Result |
|-------|--------|
| `go test ./...` (hawk) | pass |
| `go test ./...` (eyrie) | pass |
| Provider JSON sanitization | pass (`internal/config/milestone_verify_test.go`) |
| Setup flow key → model | pass (`TestVerify_EvaluateSetupFlow`) |
| Keychain-only discovery | pass (`eyrie/config/discovery_credentials_test.go`) |
| Remove credential | pass (`internal/config/credentials_store_test.go`, `cmd/chat_config_remove_test.go`) |
| Read tool path blocks | pass (`internal/tool/safety_test.go`) |
| Docker host `~/.hawk` isolation | skip (Docker not ready on verify host) |

## Iteration log

| Date | Iteration | Changes |
|------|-----------|---------|
| 2026-05-19 | 0 | Created plan; audited hawk/eyrie state |
| 2026-05-19 | 1 | setup_status, onboarding PersistAPIKey, welcome CTA, auto /config, block chat until setup |
| 2026-05-19 | 2 | Eyrie-owned credential fallback (bootstrap catalog, `HasAnyConfiguredDeployment`, placeholder filter); hawk `EvaluateSetup` |
| 2026-05-19 | 3 | Committed hawk `973671c` + eyrie `2657c72`; moved eyrie WIP off `main` onto `feature/secure-credentials-sandbox` |
| 2026-05-19 | 4 | Automated verification tests + `scripts/verify-milestone.sh`; `/sandbox` help clarified; AGENTS.md milestone section |
| 2026-05-20 | 5 | Keychain-only hardening: removed env-file credential paths, legacy API cleanup, `DiscoveryCredentials(ctx)` store-only |
| 2026-05-20 | 7 | Phase 2.4: `CatalogEmptyHint` for empty/missing catalog; verify script + AGENTS.md updated |

## Push (when ready)

```bash
# hawk
cd hawk && git push -u origin feature/secure-credentials-sandbox

# eyrie
cd eyrie && git push -u origin feature/secure-credentials-sandbox
```

## Related docs

- [`docs/SECURITY-SOLO.md`](../docs/SECURITY-SOLO.md) — solo developer security model
- [`eyrie/plans/DYNAMIC-MODEL-DISCOVERY.md`](../../eyrie/plans/DYNAMIC-MODEL-DISCOVERY.md) — discovery edge cases (§9 security updated)
