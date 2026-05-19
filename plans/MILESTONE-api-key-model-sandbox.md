# Milestone: API key → model → sandbox

**Status:** in progress (feature branch committed locally; push + CI pending)  
**Branch (both repos):** `feature/secure-credentials-sandbox`  
**Out of scope:** conversation DAG (`/fork`, `convo.db` as source of truth), langdag Go import  
**Reference layout:** herm + langdag sibling repos (already done for hawk + eyrie)

| Repo | Branch | Local commit |
|------|--------|--------------|
| hawk | `feature/secure-credentials-sandbox` | `973671c` |
| eyrie | `feature/secure-credentials-sandbox` | `2657c72` (includes `eac730b` Bedrock routing) |

`eyrie/main` is reset to `origin/main`; all WIP is on the feature branch only.

## Goal

A new user can:

1. Paste an API key securely (keychain, not `provider.json`)
2. Pick a model from eyrie discover output
3. Chat with tools running in Docker by default

## Architecture

```
User /config
    → PersistAPIKey (eyrie keychain; ValidateCredentialSecret)
    → ApplyEyrieCredentials (discover + provider.json routing only)
    → model picker (SetupUI canonical ids)
    → settings.json (model id only)

hawk chat
    → PrepareCredentialDiscovery (keychain + ~/.hawk/env)
    → EvaluateSetup (block chat if key/model missing)
    → container boot (Docker)
    → session.StreamChat via eyrie client (keys on host only)

Credential discovery (eyrie-owned, no hawk hardcoded env lists):
    catalog cache → BootstrapCatalogV1 → legacy profiles (last resort)
    → DiscoveryCredentials + HasAnyConfiguredDeployment
```

## Phases

### Phase 0 — Plan & tracking (this doc)

- [x] Write milestone plan
- [x] Keep an **Iteration log** at the bottom updated each PR/session

### Phase 1 — API keys (secure first-run)

| # | Task | Status |
|---|------|--------|
| 1.1 | `setup_status.go`: `EvaluateSetup`, `HasConfiguredDeployment`, `NeedsFirstRunSetup` | done |
| 1.2 | Onboarding `RunSetup` uses `PersistAPIKey` (not plain `SaveEnvFile` only) | done |
| 1.3 | Welcome banner shows setup CTA when keys/model missing | done |
| 1.4 | TUI auto-opens `/config` hub on first run when setup needed | done |
| 1.5 | `MigrateProviderSecrets` on every hawk start (already in root) | done |
| 1.6 | Tests: `HasConfiguredDeployment`, placeholder rejection | done |
| 1.7 | No secrets in `provider.json` on disk | done (`TestVerify_*` in `milestone_verify_test.go`) |

### Phase 2 — Model selection

| # | Task | Status |
|---|------|--------|
| 2.1 | After key: guided model picker (`configGuideAfterKey`) | done |
| 2.2 | Block chat send when no model (clear error → `/config`) | done |
| 2.3 | Catalog prefetch at startup when keys present | done |
| 2.4 | Friendly error when catalog empty (no keys / network) | partial |
| 2.5 | Setup flow: key + model clears `NeedsSetup` | done (`TestVerify_EvaluateSetupFlow`) |

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
| 4.3 | CI green on both repos | partial (local `go test ./... -short` pass; GitHub CI not run here) |
| 4.4 | Update `AGENTS.md` milestone section (not DAG) | done |

## Definition of done

- [ ] Fresh macOS: `hawk` → config opens → key → model → message works (**manual** — not run in CI agent)
- [x] `provider.json` has no API keys on disk (automated: `TestVerify_ProviderJSONOnDiskHasNoSecrets`, migrate test)
- [x] Credential files blocked from read tool (`TestIsSensitivePath` in `safety_test.go`)
- [ ] Docker running: bash in container end-to-end chat (**manual**; automated test skips when Docker unavailable)
- [x] DAG unchanged (optional `/fork` still best-effort only)

## Verification (2026-05-19)

Run locally:

```bash
./scripts/verify-milestone.sh
```

| Check | Result |
|-------|--------|
| `go test ./... -short` (hawk) | pass |
| `go test ./... -short` (eyrie) | pass |
| Provider JSON sanitization | pass (`internal/config/milestone_verify_test.go`) |
| Setup flow key → model | pass (`TestVerify_EvaluateSetupFlow`) |
| Read tool path blocks | pass (`internal/tool/safety_test.go`) |
| Docker host `~/.hawk` isolation | skip (Docker not ready on verify host) |

## Iteration log

| Date | Iteration | Changes |
|------|-----------|---------|
| 2026-05-19 | 0 | Created plan; audited hawk/eyrie/herm state |
| 2026-05-19 | 1 | setup_status, onboarding PersistAPIKey, welcome CTA, auto /config, block chat until setup |
| 2026-05-19 | 2 | Eyrie-owned credential fallback (bootstrap catalog, `HasAnyConfiguredDeployment`, placeholder filter); hawk `EvaluateSetup`; deployment UI uses keychain + env |
| 2026-05-19 | 3 | Committed hawk `973671c` + eyrie `2657c72`; moved eyrie WIP off `main` onto `feature/secure-credentials-sandbox` |
| 2026-05-19 | 4 | Automated verification tests + `scripts/verify-milestone.sh`; `/sandbox` help clarified; AGENTS.md milestone section |

## Push (when ready)

```bash
# hawk
cd hawk && git push -u origin feature/secure-credentials-sandbox

# eyrie
cd eyrie && git push -u origin feature/secure-credentials-sandbox
```
