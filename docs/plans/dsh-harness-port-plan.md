# Adopt durable-design ideas from deepseek-harness in hawk

Status: Phase 0 scaffolded on `feat/dsh-harness-port-p0-eventlog`.

## Principles

- Port **protocols and invariants**, never Cordis (no DI framework, no `ctx.effect`, no declaration merging).
- Everything is Go-native: interfaces + a small registry + typed events, wired at the composition root.
- Honor `docs/session-decomposition.md`: this plan is the extension mechanism that lets
  `agentLoop` shrink to ~250 lines.

## Reference sources

| dsh file | Extract | Maps to hawk |
| --- | --- | --- |
| `packages/core/session/src/index.ts` | append-only `SessionEvent` log + `session/event` emit | `internal/eventlog` |
| `docs/architecture.md` "Turn flow" | `deriveMessages`; "model-visible ⟺ logged" | `Session.Persistence()` projection |
| `packages/core/tools/src/index.ts` | `tools/pre-execute` / `tools/execute` / `tools/post-execute` waterfall | `internal/tool/interceptor.go` (Phase 1) |
| `packages/interaction/user-approval/src/index.ts:30` | `approval/request` waterfall, fail-closed | `internal/engine/approval_gate.go` (Phase 1) |
| `docs/cordis-primer.md` "Waterfall Semantics" | `next()` delegate / short-circuit | interceptor contract |
| `docs/capability-seams.md` | owner / impl / consumer roles | `docs/architecture/hawk-capability-seams.md` (Phase 2) |

## Phase 0 — Event-sourced session log + "model-visible ⟺ logged"

Delivered in this branch: `internal/eventlog` package.

- `Event` / `Type`: append-only vocabulary (`session.meta`, `message.user`,
  `message.assistant`, `tool.call`, `tool.result`, `context.injected`,
  `session.compacted`).
- `Log`: monotonic `Seq` assignment, by-type index, observer callback.
- `invariants.go`: `Validate` enforces known types + monotonic sequence (fail-loud on load).
- `wire.go`: `MarshalWire` / `DecodeWire` for the persisted shape.

Remaining (subsequent PRs, each gated):

1. ~~Project `Session.Persistence().RawMessages()` through `DeriveMessages`; replace
   direct `SetRawMessages(append(...))` in `agentLoop`/`ToolService` with
   journaled append.~~ Delivered: `adduser`/`AddAssistant` and the agent loop's
   model-visible appends (cache hit, max-tokens recovery, final text, tool-call,
   tool-result, steering, vision attachment) now route through the journaled
   append methods, and every `Session` materializes an `eventlog.Log` at
   construction. `PersistenceService.Reconstructible()` now asserts the strict
   "model-visible ⟺ logged" invariant; the reconstructible assert is delivered.
2. ~~Persist the event log behind `SESSION_FORMAT_VERSION == 1`.~~ Delivered:
   `session.Session.Events` (`[]eventlog.WireEvent`), `Save` writes event lines
   plus `format_version=1` in meta when present, `scanJSONLLines`/`loadJSONLFile`
   decode and validate them, and `engine.Session.JournalWire()` exports the spine
   for `persistDaemonSession`. Loaded spines are replayed into the engine journal
   on resume via `eventlog.Rehydrate` + `Session.ReplayJournal`; version `0` stays
   byte-compatible.

## Phase 1 — Typed tool-pipeline interceptors + fail-closed approval

- `internal/tool/interceptor.go`: `InterceptFn`, `Stage`, `ChainNode`/`Chain`.
- `DefaultToolPipeline()` on `ToolService`; default nodes preserve current behavior
  (permission → blast-radius → trace; timeout/retry/exec; redact/loop-detect/memory-distill).
- `EventBus.Waterfall` (ordered, short-circuit, returns a value) + `Dispose`.
- `approval.request` waterfall; no answerer = deny (fail-closed).

## Phase 2 — Seam discipline (docs + disposers)

- `docs/architecture/hawk-capability-seams.md`: owner / impl / consumer table.
- Registrations return disposers (tool registry, hooks, MCP client).
- "Where new behavior goes" table.

## Phase 3 — Titles + plan-mode-as-state (optional)

- Log-backed session titles with one provider spot + deterministic fallback.
- `internal/spec` plan state as logged facts.

## Gates

Each phase: `make ci` + `hawk verify`; no direct commits to `main`.
