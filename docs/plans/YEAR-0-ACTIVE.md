# Year 0 Active Track (Grok → Hawk)

**Status:** Active  
**Date:** 2026-07-16  
**ADR:** [ADR-0003](../architecture/adr/ADR-0003-grok-behavioral-port-go-multirepo.md)  
**Full matrices:** [FULL-GROK-ECO-TO-HAWK-ECO-PORT-PLAN.md](./FULL-GROK-ECO-TO-HAWK-ECO-PORT-PLAN.md),
[GROK-CLASS-CAPABILITY-LONG-HORIZON-PLAN.md](./GROK-CLASS-CAPABILITY-LONG-HORIZON-PLAN.md)  
**Call-site inventory:** [Y0-CALL-SITE-INVENTORY.md](./Y0-CALL-SITE-INVENTORY.md)

This is the **executable Year 0 program**. It does not replace the full
port matrices; it freezes what “Year 0 done” means and tracks pack status.

## Port meaning

| Do | Do not |
|----|--------|
| Reimplement Grok **behavior** in Go | Copy Rust crates or depend on Grok |
| Map capabilities to hawk-eco repos | Collapse engines into hawk monorepo |
| Wire existing modes/budgets first | Rebuild eyrie/yaad/tok as Grok clones |
| Privacy-first telemetry (OTEL opt-in) | Port Mixpanel defaults |

## Pack status

| Pack | Scope | Status |
|------|--------|--------|
| PACK-00 | ADR, inventory, flags, test matrix template | **Done** (2026-07-16) |
| PACK-01 | `hawk-core-contracts/agent` spawn DTOs | **Done** (v0.1.6) |
| PACK-02 | Typed spawn + Agent tool + taskruntime unify | **Mostly done** — typed Agent tool, explore bash hard gate, taskruntime registry, worktree isolation; true transcript resume still stub |
| PACK-03 | sandbox.toml + folder trust + safe-bash | **Partial** — folder trust + sandbox.toml + project plugin/hook gates; named acceptEdits modes / safe-bash product polish remain |
| PACK-04 | Hooks complete + PreToolUse in PermissionEngine | **Done** — PreToolUse deny-before-autonomy, vendor aliases, HTTP hooks, discovery dirs, plugin env, acceptEdits/dontAsk aliases |
| PACK-05 | Multi-component plugins + marketplace MVP | Pending |
| PACK-06 | Monitor / Wait / Kill / `/loop` | Pending |
| PACK-07 | Structured AskUser + plan/spec align | Pending |
| PACK-08 | Crash, announcements, prompt queue, interjection | Pending |
| Docs 01–12 | `docs/user-guide/` as packs land | Parallel |

## Year 0 exit criteria

- [x] Model can spawn `explore` \| `plan` \| `general-purpose` via Agent tool schema (isolation=worktree works; resume still stub)
- [x] Explore bash cannot mutate (`ExploreBashAllowed` segment allowlist + `ReadOnlyBash` on subagents)
- [x] Unified agent taskruntime (`internal/taskruntime`; shell TaskOutput merge is PACK-06)
- [x] Folder trust gates project hooks / plugins (`.hawk/plugins`, `.hawk/hooks`; MCP/LSP follow same AllowLoadPath)
- [x] `sandbox.toml` profiles + project additive merge; deny globs fail-closed
- [x] PreToolUse hooks can deny inside `PermissionEngine` before autonomy
- [ ] Multi-component plugins + marketplace MVP install path
- [ ] Monitor + Wait/Kill + `/loop`; structured AskUser; plan/spec aligned
- [ ] Crash handler, announcements, prompt queue, interjection
- [ ] User-guide docs `01`–`12` under `hawk/docs/user-guide/`
- [x] ADR-0003 published
- [x] PACK-00 inventory + flags + spawn matrix template complete

## Explicit deferrals (not Year 0 exit)

ACP phase-2 depth, full SDK spawn surface, signed enterprise policy E2E,
foreign session import, hunk tracker / CoW worktrees, mermaid/media/video,
computer hub, full slash/TUI pixel parity, Mixpanel.

## Feature flags (env)

| Env | Default | Purpose |
|-----|---------|---------|
| `HAWK_Y0_SPAWN_V2` | `1` once PACK-02 ships; `0` during dual path | Typed SpawnRequest path |
| `HAWK_Y0_FOLDER_TRUST` | `1` recommended after PACK-03 | Gate project automation |
| `HAWK_Y0_MARKETPLACE` | `0` until PACK-05 + trust | Marketplace install path |

Implementation: `internal/flags/y0.go`.

## Order rule

```text
contracts → spawn/taskruntime → trust/sandbox → hooks-first → plugins → tasks/UX
```

Do not reverse. No marketplace auto-load before folder trust.
