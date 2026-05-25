# Developer path — research & plan

**Audience:** One developer on macOS or Linux running hawk locally (no Vault, no team proxy).  
**Security reference:** [`docs/SECURITY-DEVELOPER.md`](../docs/SECURITY-DEVELOPER.md)  
**Product direction:** [`docs/DEVELOPER-PATH.md`](../docs/DEVELOPER-PATH.md)  
**Related milestone:** [`MILESTONE-api-key-model-sandbox.md`](MILESTONE-api-key-model-sandbox.md)

## Executive summary

Hawk’s core loop (eyrie → tok → tools → yaad) is **production-capable for individual developers**. What was missing was a **single, honest readiness report** that ties setup, security, sandbox, and ecosystem together.

This plan adds **`hawk path`** and **`/path`** as the canonical “am I good?” check on the developer path.

---

## Deep research — current state (May 2026)

### Architecture (what works)

| Layer | Role | Required? | Path fit |
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

| Gap | Severity | Resolution |
|-----|----------|------------|
| No unified readiness command | High | `hawk path` + `FormatDeveloperPathReport()` |
| Setup scattered across doctor/preflight/credentials | Medium | Report sections: Setup · Security · Sandbox · Ecosystem |
| Milestone verify script not in CI smoke | Medium | `scripts/verify-developer-path.sh` + smoke extension |
| yaad auto-remember too noisy/heuristic | Low | `memory.ShouldAutoRemember()` |
| `hawk sandbox` = diff sandbox, not Docker | Confusing | Report names “Docker isolation” explicitly |
| Conversation DAG | Out of scope | Documented as future; `/fork` stays best-effort |

---

## Plan — phases

### Phase 1 — Readiness report

1. `internal/config/path_path.go` — structured checks + `FormatDeveloperPathReport()`
2. `hawk path` — human report; exit 1 if chat not ready
3. TUI `/path` — same report in chat

### Phase 2 — Verification

1. `scripts/verify-developer-path.sh` — milestone + path tests
2. Extend `scripts/smoke-hawk.sh` with `hawk path`
3. `make path` target

### Phase 3 — Onboarding copy

1. Welcome + setup hints (keychain, `/config`, `/path`)
2. Auto-open `/config` on first run

---

## Verification matrix

```bash
make path                       # verify-developer-path.sh
./scripts/smoke-hawk.sh
hawk path
hawk preflight
hawk doctor
```

---

## Iteration log

| Date | Change |
|------|--------|
| 2026-05-24 | Initial research doc; `hawk path`, verify script, docs |
| 2026-05-24 | Rename user-facing copy to “developer path” (not “individual developer”) |
