# Solo developer path — research & plan

**Audience:** One developer on macOS or Linux running hawk locally (no Vault, no team proxy).  
**Security reference:** [`docs/SECURITY-SOLO.md`](../docs/SECURITY-SOLO.md)  
**Related milestone:** [`MILESTONE-api-key-model-sandbox.md`](MILESTONE-api-key-model-sandbox.md)

## Executive summary

Hawk’s core loop (eyrie → tok → tools → yaad) is **production-capable for solo dev**. What was missing was a **single, honest readiness report** that ties setup, security, sandbox, and ecosystem together — instead of asking the developer to run `doctor`, `preflight`, `credentials status`, and read three docs.

This plan adds **`hawk solo`** and **`/solo`** as the canonical “am I good?” check for the solo path.

---

## Deep research — current state (May 2026)

### Architecture (what works)

| Layer | Role | Required? | Solo fit |
|-------|------|-----------|----------|
| **eyrie** | LLM client, catalog, OS keychain credentials, routing | Yes | Keys never in JSON; chat uses model id only |
| **tok** | Token estimate + fast context compression | Yes (embedded) | No config; used in compaction + budget UI |
| **yaad** | SQLite graph at `~/.yaad/data/yaad.db` | No | Graceful degrade; recall + CoreMemory tools |
| **hawk** | Agent loop, 40+ tools, Docker bash default | Yes | TUI `/config` first-run, container isolation |

Message flow (see [`docs/ecosystem-message-flow.md`](../docs/ecosystem-message-flow.md)):

```
/config (key → model) → StreamChat → yaad recall → eyrie stream → tools → yaad remember → tok compress
```

### Security model (implemented on `main`)

- API keys: OS secret store only (`PersistAPIKey`, `credentials.LookupSecret`)
- `provider.json`: routing metadata only; `MigrateProviderSecrets()` strips disk secrets
- Legacy `~/.hawk/env` / `.env`: one-time migrate → keychain → delete
- Read tool: blocks `~/.hawk/provider.json`, env files, `~/.ssh/*` (`internal/tool/safety.go`)
- Bash: Docker container when daemon available (`shouldUseContainer()` default true)

### Gap analysis

| Gap | Severity | Resolution in this plan |
|-----|----------|-------------------------|
| No unified “solo ready” command | High | `hawk solo` + `FormatSoloPathReport()` |
| Setup scattered across doctor/preflight/credentials | Medium | Solo report sections: Setup · Security · Sandbox · Ecosystem |
| Milestone verify script not in CI smoke | Medium | `scripts/verify-solo-path.sh` + smoke extension |
| yaad auto-remember too noisy/heuristic | Low | `memory.ShouldAutoRemember()` — 2+ triggers, explicit decision lines |
| `hawk sandbox` = diff sandbox, not Docker | Confusing | Solo report names “Docker isolation” explicitly |
| Conversation DAG | Out of scope | Documented as future; `/fork` stays best-effort |
| Fresh-macOS manual E2E | Medium | `scripts/e2e-macos.sh` referenced; not blocking CI |

### What “fully handles everything” does *not* mean

- Team-shared memory, cloud sync, or multi-tenant keys
- Perfect semantic memory (yaad remains local + heuristic auto-remember)
- Linux-only landlock on macOS (Docker or host fallback)
- Conversation DAG as source of truth

---

## Plan — phases

### Phase 1 — Solo path report (this PR)

1. `internal/config/solo_path.go` — structured checks + `FormatSoloPathReport()`
2. `hawk solo` — human report; exit 1 if chat not ready
3. TUI `/solo` — same report in chat
4. Unit tests with isolated `HOME`

**Checks:**

| Section | Check | Pass | Warn | Fail |
|---------|-------|------|------|------|
| Setup | credentials | keychain has deployment | — | none |
| Setup | model | selected in provider.json | — | none |
| Setup | catalog | models cached | missing / empty hint | — |
| Security | keychain | writable | read-only detail | — |
| Security | provider.json | no secrets on disk | — | api_key etc. present |
| Security | legacy env | no ~/.hawk/env files | files still present | — |
| Security | read guard | sensitive paths blocked | — | — |
| Sandbox | docker | daemon up | bash on host | — |
| Ecosystem | eyrie | preflight ready | partial | credentials/model fail |
| Ecosystem | yaad | bridge ready | not initialized | — |
| Ecosystem | tok | embedded OK | — | — |

**Ready flags:**

- `ChatReady` — credentials + model + eyrie preflight (or equivalent)
- `SecureReady` — no disk secrets, keychain OK, read guard OK
- `Ready` — `ChatReady && SecureReady` (Docker warn does not block)

### Phase 2 — Verification

1. `scripts/verify-solo-path.sh` — milestone tests + solo path tests
2. Extend `scripts/smoke-hawk.sh` with `hawk solo`
3. CI smoke job runs verify-solo-path

### Phase 3 — Memory quality (small)

1. `memory.ShouldAutoRemember()` — shared logic for stream auto-remember
2. Remember explicit `Decision:` / `always use` / `never use` lines with category hints

### Phase 4 — Docs

1. README “Solo developer path” section
2. Update milestone iteration log
3. AGENTS.md pointer to `hawk solo`

---

## Verification matrix

```bash
./scripts/verify-solo-path.sh   # unit + milestone checks
./scripts/smoke-hawk.sh         # build + CLI smoke
hawk solo                       # human report
hawk preflight                  # eyrie-only (subset)
hawk doctor                     # full diagnostics
make smoke                      # alias
```

CI: smoke job must pass (includes solo path tests).

---

## Iteration log

| Date | Change |
|------|--------|
| 2026-05-24 | Initial research doc; implement Phase 1–4 |
