# Milestone: API key → model → sandbox

**Status:** in progress  
**Out of scope:** conversation DAG (`/fork`, `convo.db` as source of truth), langdag Go import  
**Reference layout:** herm + langdag sibling repos (already done for hawk + eyrie)

## Goal

A new user can:

1. Paste an API key securely (keychain, not `provider.json`)
2. Pick a model from eyrie discover output
3. Chat with tools running in Docker by default

## Architecture

```
User /config
    → PersistAPIKey (eyrie keychain)
    → ApplyEyrieCredentials (discover + provider.json routing only)
    → model picker (SetupUI canonical ids)
    → settings.json (model id only)

hawk chat
    → PrepareCredentialDiscovery
    → container boot (Docker)
    → session.StreamChat via eyrie client (keys on host only)
```

## Phases

### Phase 0 — Plan & tracking (this doc)

- [x] Write milestone plan
- [ ] Keep an **Iteration log** at the bottom updated each PR/session

### Phase 1 — API keys (secure first-run)

| # | Task | Status |
|---|------|--------|
| 1.1 | `setup_status.go`: `HasConfiguredDeployment`, `NeedsFirstRunSetup` | done |
| 1.2 | Onboarding `RunSetup` uses `PersistAPIKey` (not plain `SaveEnvFile` only) | done |
| 1.3 | Welcome banner shows setup CTA when keys/model missing | done |
| 1.4 | TUI auto-opens `/config` hub on first run when setup needed | done |
| 1.5 | `MigrateProviderSecrets` on every hawk start (already in root) | done |
| 1.6 | Tests: `HasConfiguredDeployment` with mock env | pending |
| 1.7 | Manual: paste key → no secret in `provider.json` | pending |

### Phase 2 — Model selection

| # | Task | Status |
|---|------|--------|
| 2.1 | After key: guided model picker (`configGuideAfterKey`) | done (WIP branch) |
| 2.2 | Block chat send when no model (clear error → `/config`) | done |
| 2.3 | Catalog prefetch at startup when keys present | done |
| 2.4 | Friendly error when catalog empty (no keys / network) | partial |
| 2.5 | Manual: key → model → first message succeeds | pending |

### Phase 3 — Sandbox

| # | Task | Status |
|---|------|--------|
| 3.1 | Container default on (`shouldUseContainer`) | done |
| 3.2 | Block input when container required but Docker down | done |
| 3.3 | `ContainerExecutor` wired for bash | done |
| 3.4 | Read tool blocks credential paths (`safety.go`) | done (WIP) |
| 3.5 | Document `--no-container` vs secure mode | done (`SECURITY-SOLO.md`) |
| 3.6 | Integration test or script: bash cannot read `~/.hawk/env` | pending |
| 3.7 | Clarify `/sandbox` vs default container in help | pending |

### Phase 4 — Hardening & ship

| # | Task | Status |
|---|------|--------|
| 4.1 | Commit hawk `feature/secure-credentials-sandbox` | pending |
| 4.2 | Commit matching eyrie credential/catalog changes | pending |
| 4.3 | CI green on both repos | pending |
| 4.4 | Update `AGENTS.md` milestone section (not DAG) | pending |

## Definition of done

- [ ] Fresh macOS: `hawk` → config opens → key → model → message works
- [ ] `provider.json` has no API keys on disk
- [ ] Docker running: bash runs in container; credential files blocked from read tool
- [ ] DAG unchanged (optional `/fork` still best-effort only)

## Iteration log

| Date | Iteration | Changes |
|------|-----------|---------|
| 2026-05-19 | 0 | Created plan; audited hawk/eyrie/herm state |
| 2026-05-19 | 1 | setup_status, onboarding PersistAPIKey, welcome CTA, auto /config, block chat until setup |
