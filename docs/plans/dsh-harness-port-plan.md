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

Delivered in this branch on top of Phase 0:

- `internal/tool/interceptor.go`: `Stage`, `ToolRequest`/`ToolResult`, `InterceptFn`,
  `Chain`, and `Pipeline` with registration-ordered waterfall semantics and disposers.
- `internal/tool/interceptor_test.go`: order, short-circuit, head/tail removal,
  stage isolation, fail-closed deny, and disposer behavior.
- `internal/engine/tool_service.go`: `ToolService.Pipeline()`/`SetPipeline` and a
  `StagePreExecute` run inside `ExecuteOne` after the permission engine and before
  approval/execution. `ShortCircuitDeny` stops the call; non-`ShortCircuit` errors
  are treated as fail-closed denials.
- `internal/engine/stream_tool_exec_test.go`: integration tests proving the wired
  path stops execution and that an empty pipeline is a strict pass-through.
- `internal/engine/event_bus.go`: ordered, value-returning `Waterfall` handlers plus
  `RunWaterfall` (separate from the existing pub/sub `Subscribe`/`Publish` path).
- `internal/engine/approval_waterfall.go` + test: `approval.request` waterfall
  with "no answerer = deny" (fail-closed).

GitNexus `detect-changes` flags CRITICAL risk on `EventBus` because it is a
high-degree node; the changed symbols are all additive (`waterfall*`), and no
pre-existing caller or execution path is altered.

Remaining for Phase 1 (subsequent PRs, each gated):

- ~~Wire `DefaultToolPipeline()` at the composition root:~~ Delivered:
  `internal/engine/tool_pipeline.go` exposes `DefaultToolPipeline()` as the
  empty, strict pass-through seam and `NewSessionWithClient` installs it via
  `SetPipeline`.
- ~~Route the post-execute stage through `PostProcess`~~ Delivered: the stage runs
  after `NormalizeOutput` and before the legacy lifecycle pipeline; an interceptor
  may mutate or replace the normalized result, and `*tool.ShortCircuit` overrides
  output/isErr.
- ~~ApprovalGate dispatch through `ApprovalWaterfall`.~~ Delivered: `ApprovalGate`
  gained a `Waterfall` field and `PermissionService.CheckApproval` consults it
  first (additive); all-abstain and empty chains deny (fail-closed).

Phase 1 follow-ups are delivered on this branch. Note: the post-execute
`ToolRequest.Tool` is nil at `PostProcess` because the resolved tool is not yet
carried through to that stage; pre-execute has it. If a post-stage needs the
concrete tool, carry it through `toolExecResult` in a later PR.

## Phase 2 — Seam discipline (docs + disposers)

- `docs/architecture/hawk-capability-seams.md`: owner / impl / consumer table.
- Registrations return disposers (tool registry, hooks, MCP client).
- "Where new behavior goes" table.

## Phase 3 — Titles + plan-mode-as-state (optional)

- ~~Log-backed session titles with one provider spot + deterministic fallback.~~
  Delivered: `eventlog.TitleFromMessages` + `PersistenceService.JournalTitle`;
  the daemon's `persistDaemonSession` fills `session.Session.Name` from
  `sess.JournalTitle()` when no human-supplied title exists.
- ~~`internal/spec` plan state as logged facts.~~ Delivered: `eventlog.SpecState`
  / `SpecFact` vocabulary + wire decode; `PermissionService.AdvanceSpecStage`
  appends a durable fact through the session journal.

## Gates

Each phase: `make ci` + `hawk verify`; no direct commits to `main`.
