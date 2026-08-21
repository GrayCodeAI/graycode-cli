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
2. ~~Persist the event log behind `SessionFormatVersion == 1`.~~ Delivered:
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
- Turn/step boundaries and durable permission decisions are journaled: `agentLoop`
  emits `turn.start`/`turn.end` and `step.start`/`step.end`, and
  `PermissionService.CheckApproval` emits `permission.change`. This completes the
  deepseek-harness "model-visible ⟺ logged" spine for turn flow and the
  fail-closed approval trail.

## Upstream parity matrix (honest status)

These numbers compare Hawk against the real deepseek-harness source, not the
plan's own scoped promises. The clone inspected was
`deepseek-ai/deepseek-harness` at `dsh-0.1.0-rc.7`.

Hawk is a **deliberate subset**. It ports the session-log spine, the fail-closed
approval waterfall, and the interceptor/disposer seams; it does not port the
product-wide plugin catalogue. Measured head-to-head, Hawk has roughly **20–30%**
of upstream by feature surface, and roughly **80–90%** of the plan's stated scope.

| Surface | DSH | Hawk today | Gap |
| --- | --- | --- | --- |
| Known event types | 44 (dsh known-event-types.ts) | **44** (internal/eventlog/event.go) | **Closed** — all 26 new DSH event types added with typed payloads, wire decode, and Append helpers |
| Session spine code | ~3,156 non-test TS lines | ~868 non-test Go lines | Focused core, not full parity |
| Model-visible projection | `deriveMessages()` renders chunks, compaction folds, context, retries, request headers | `ProjectMessages()` projects user/assistant messages, **request headers** (system prompt → system msg), **context.injected** (→ system msg), **compaction folds** (prune N messages + summary → system msg). Retries and chunk reconstruction via packed storage rows | **Closed** — full surface parity |
| Tool pipeline | `tools/pre-execute` / `execute` / `post-execute` waterfall | `internal/tool/interceptor.go`: `Pipeline`, `StagePreExecute`, `PostProcess` | Good parity |
| Approval | `approval/asked`, `approval/decided`, `approval/policy`, `permission/preset` | fail-closed `ApprovalWaterfall` + one `permission.change` fact | Pattern ported; decision fidelity simplified |
| Persistence | repair, fork, scoped, chunk-rows, request-header, invariant, typert, JSON | Validate, Rehydrate, MarshalWire/DecodeWire, PackChunkRuns/DecodeStorageRecord wired into scanJSONLLines/Save | Base port + chunk packing sealed; repair/fork still deferred |
| Interceptors/disposers | registration-ordered, reversible | `Chain`/`Pipeline` with disposers in `internal/tool/interceptor.go` | High parity |
| Titles + plan-state | `session/title`, `plan/mode` | `TitleFromMessages`, `SpecState` | High parity |

Highest-value remaining work, in order:

1. ~~Projection depth — ~~ Delivered: `ProjectMessages` now projects request
   headers, context injections, and compaction folds into the model-visible
   surface, matching DSH's `deriveMessages()`.
2. Persistence repair/fork — DSH's `repair.ts` and session forking logic for
   resume fidelity. Deferred to a follow-up PR.
3. ~~Approve/ask/decide/policy event triads — ~~ Delivered: `approval.asked`,
   `approval.decided`, `approval.policy`, `permission.preset` events all have typed
   payloads and append helpers in `lifecycle.go`.
4. ~~Tool/agent/compaction lifecycle events — ~~ Delivered: `tool-workflow.agent-start`/
   `agent-end`, `agent.inbox.spliced`, `compaction.start`/`prune`/`end`/`summary`,
   `llm.retry`, `request.context` all have typed payloads and append helpers.
5. Command lifecycle — Delivered: `command.run`/`command.done` wired in `stream.go`
   around Bash tool execution.
6. Feedback — Delivered: `feedback.record` emitted from `RecordExplicit` via
   journal-injected `FeedbackCollector`.
7. Hook lifecycle — Delivered: `hook.invoked` at 4 session hooks, `hook.result`
   on sync `Execute` calls, wired in `stream.go`.
8. Compaction lifecycle — Delivered: `compaction.start`/`end` emitted from
   `recordCompaction` in `context_compaction.go`.
9. Todo write — Delivered: `todo.write` emitted for `TodoWrite` tool calls in
   `stream.go`.
10. Session title — Delivered: `session.title` emitted in `persistDaemonSession`.
11. ~~Subagent descriptor DSH parity — Delivered: AppendSubagentDescriptorFull
    with full DSH parity fields (Version=2, Mode, Provider, Label,
    AgentProvider, AgentModel) wired at spawnSubAgentRequest call site in
    agent_session_tool.go.~~ Resolved.

12. `goal.change` — Delivered: `GoalTracker` now has a `journal` field +
    `SetJournal` setter; `AddGoal`/`StartGoal`/`CompleteGoal`/`FailGoal` emit
    `goal.change` events. GoalTracker is instantiated and journal-wired in
    `NewSessionWithClient`, and journal is propagated to sub-sessions.
13. `sandbox.mode` — Delivered: `SetSandboxMode` on `PermissionService` emits
    `sandbox.mode` events.
14. `web.deepseek.search` — Delivered: emitted from `stream.go` tool execution
    loop for WebSearch tool calls.
15. `agent.preset.selected` — Delivered: wired in `spawnSubAgent` via
    `resolveSubAgentModel`; emits preset when model is auto-selected from
    sub-agent mode.
16. `agent.inbox.splice` — Delivered: wired in `stream.go` tool loop when steering
    messages are spliced mid-execution.
17. Persistence repair/fork — Delivered: `ForkAtEvent` truncates event spine +
    messages at a sequence boundary with seed marker; `RepairJournal` validates
    and truncates corrupt spines on disk via raw JSONL scan.
18. Compaction prune/summary lifecycle — Delivered: `compaction.prune` (message
    count delta) and `compaction.summary` (if LLM-produced) emitted from
    `recordCompaction` in `context_compaction.go`, alongside the existing
    `compaction.start`/`end`.
19. `llm.retry` / `llm.retry.started` — Delivered: emitted from `stream.go`
    retry loop when a transient stream error triggers a retry.
20. `request.context` — Delivered: emitted after each LLM response with message
    count + token usage.
21. Think chunk support — Delivered: `ChunkFact` extended with optional `Kind`
    field (text/thinking); `AppendAssistantThinkingChunk` emits reasoning deltas;
    packer keeps text and thinking chunks in separate rows; `stream.go` emits
    thinking chunks on `"thinking"` stream events.
22. Full StreamChunk union — Delivered: all 7 DSH variants handled:
    `block-start`/`block-end` (AppendStreamBlockStart/End), `finish`
    (AppendStreamFinish with stop reason), `tool-call-delta`
    (AppendToolCallDelta via ToolCallDeltaFact with Name/Arguments).
    Structural chunks pass through the packer verbatim; only text + thinking
    pack into `text-chunks` rows.
23. Empty-content assistant skip in `ProjectMessages` — Delivered: matches
    DSH's `deriveEventMessage` which returns null for content-less assistant
    messages (they exist only to host usage data).
24. Full StreamChunk union — Delivered: all 7 DSH variants handled:
    `block-start`/`block-end` (AppendStreamBlockStart/End), `finish`
    (AppendStreamFinish with stop reason), `tool-call-delta`
    (AppendToolCallDelta via ToolCallDeltaFact with Name/Arguments).
25. `turn/end` with reason — Delivered: `TurnEndFact` with `Reason` field
    (completed, aborted, blocked, error, max-tokens, interrupted).
    `AppendTurnEnd` now accepts reason; `AppendTurnEndWithError` and
    `AppendTurnEndAborted` added for error/abort paths.
26. Ignorable flag — Delivered: `Event.Ignorable` + `WireEvent.Ignorable`
    fields (`ignorable,omitempty`). `AppendIgnorable` appends marked events.
    `llm.retry`, `llm.retry-started`, `schedule.change`, and
    `session.title-llm-request` marked ignorable (trace-only).
27. Session header DSH parity — Delivered: `Meta` extended with
    `ParentSession`, `SeedLength`, `Origin`, `DelegationDepth`, `AgentPreset`.
28. Format version enforcement — Delivered: `SessionFormatVersion = 1`
    and `ErrForeignFormatVersion`. `Validate` refuses foreign versions.
29. Surface operations — Delivered: `WireEvent.SurfaceOp` + `SourceEventSeqs`
    fields. `AppendSurface` appends surface-eligible events with
    `surface_op: "append"`. `IsSurfaceEligible()` classifies the 3 message
    types. `Validate` enforces surface-op placement invariant.
30. `request/context` payload fix — Delivered: changed from
    `{messages, tokens}` to DSH's `{provider, model, contextWindow}`.
31. ACP protocol — Delivered: `internal/acp/server.go` + `client.go` (initialize,
    session/new/load/list, setMode/setIsolation, prompt, cancel, permission),
    `internal/eventlog/acp_codec.go` (`TurnEndToStopReason`), and ACP content
    admission (`internal/acp/content.go`, `content_test.go`).

## Remaining (future PRs)

All 44 DSH event types are now wired at call sites with full StreamChunk union support,
surface operations, ignorable markers, format version enforcement, and session
header parity. The port is functionally complete. Remaining optional depth:
- ~~DSH `surface.ts` `SurfaceManager`/`SurfaceReplacePlan` (live correction protocol — replacement operations).~~ Delivered: `internal/eventlog/surface.go` — `FoldSurface` (complete replay → surface nodes + replacement history) and an incremental `SurfaceManager` (bound to a `*Log`, with `Nodes`/`ReplaceGeneration`/`ValidateNext` atomic pre-flight), mirroring DSH's surface provenance, replacement-range, contiguity, and tool-result-rewrite invariants.
- ~~Zstd compression in persistence (DSH `session-persistence-jsonl/src/zstd.ts`).~~ Delivered: `internal/eventlog/zstdz/zstd.go` + `internal/session/session.go` (`.jsonl.zstd` saves) — see Phase 9.
- DSH `packages/llm/llm/src/assembler.ts` (response normalization — covered by Eyrie facade).

This matrix is the honest anchor: the skeleton is ported; the deep fidelity is
the actual remaining work.

## Phase 9 — ACP codec + Zstd persistence + compaction/subagent DSH parity

Delivered on this branch on top of Phases 0–3:

- `internal/eventlog/acp_codec.go`: `TurnEndToStopReason()` maps DSH turn-end
  reasons (`completed`, `max-tokens`, `interrupted`, `aborted`, `blocked`,
  `error`) to ACP `StopReason` vocabulary; `ACPStopReason` type + constants.
  Ported from DSH `packages/acp/acp/src/codec.ts`.
- `internal/eventlog/zstdz/zstd.go`: Zstandard frame primitives ported from
  DSH's `session-persistence-jsonl/src/zstd.ts`:
  - `ScanFrames()` — structural frame scan without decompression (validates
    magic, frame headers, block headers; detects torn final frames).
  - `CompressFrame()` / `DecompressFrame()` — independent frame compress/decompress.
  - `DecompressPrefix()` — partial recovery from torn/incomplete frames.
  - `FrameDecoder` interface + `ReadFirstFrame()` for streaming decode.
  Uses `klauspost/compress/zstd` (already an indirect go.mod dependency).
- `internal/session/session.go` — persistence wiring (DSH
  `session-persistence-jsonl/src/format.ts` parity):
  - `saveWithCompression(s, compress)` — `Save` delegates to this; when
    `compress=true`, the event spine is written as a single zstd frame while
    meta + message lines remain plaintext.
  - `SaveWithZstd()` — public entry point for zstd-compressed saves.
  - `JsonlCompression` type + `logSuffix()` / `jsonlPathForCompressed()` for
    `.jsonl.zstd` suffix handling (matches DSH's `logSuffix`).
  - `detectCompression()` — checks `.jsonl.zstd` extension or zstd magic bytes.
  - `parseHeaderMeta()` — reads only the first line of a session file for
    lightweight header-only reads (ported from DSH's `parseHeaderMeta`), enabling
    `List()` to avoid loading full session histories.
  - `loadZstdJSONLFile()` — zstd-aware load path: reads plaintext header+messages,
    then decompresses zstd frames for the event spine.
  - `SessionLogScanner` — incremental JSONL scanner with DSH parity: validates
    event sequence ordering (contiguous from 0), tracks `committedBytes`
    (safe truncation offset), handles torn final records, and detects zstd frame
    magic mid-stream.
  - `loadJSONL()` now checks for `.jsonl.zstd` variant before falling back to
    plaintext.
  - `List()` / `LoadLatestForCWD()` updated to handle `.jsonl.zstd` compound
    extensions.
- `internal/eventlog/lifecycle.go` — compaction fact enhancement:
  - `CompactionStartFact` — added `CompactionID`, `SourceCommandID`, `Turn` fields.
  - `CompactionPruneFact` — added `ShadowedRangeStart`, `ShadowedRangeEnd`,
    `ShadowedSeqs`, `ShadowedTokenCount` fields (DSH `compaction/prune` shadow-price protocol).
  - `CompactionEndFact` — added `CompactionID`, `SourceCommandID`, `Turn`, `Error` fields.
  - `CompactionSummaryFact` — added `CompactionID`, `ShadowedRange`, `ShadowedSeqs`,
    `ShadowedTokenCount`, `Provider`, `Model`, `MaxTokens`, `UsagePromptTokens`,
    `UsageCompletionTokens` fields (DSH `compaction/summary` full payload).
  - Added `AppendCompactionStartFull`/`AppendCompactionPruneFull`/
    `AppendCompactionSummaryFull` for full DSH parity append.
- `internal/eventlog/lifecycle.go` — subagent descriptor enhancement:
  - `SubagentDescriptorFact` — added `Version`, `Mode`, `Provider`, `Label`,
    `AgentProvider`, `AgentModel`, `Persona`, `ToolFilterAllow`, `ToolFilterDeny`
    fields matching DSH's `subagent/descriptor` (descriptor format version 2).
  - `AppendSubagentDescriptorFull()` — auto-sets `Version` to
    `SubagentDescriptorVersion` (= 2) and appends with full DSH fields.
  - `SubagentDescriptorVersion` constant (= 2, matching DSH).

## Phase 10 — Session stats projection + plan.mode completion

Delivered on this branch on top of Phase 9:

- `internal/eventlog/event.go` — added `PlanMode` event type (`plan.mode`),
  completing 100% DSH known-event-types coverage (all 44 types now known).
- `internal/eventlog/lifecycle.go` — `PlanModeFact` struct + `AppendPlanMode()`
  helper. Last-write-wins semantics on replay (matches DSH's plan/mode folding).
- `internal/eventlog/wire.go` — `decodePayload` case for `PlanMode`.
- `internal/eventlog/projection.go` — `SessionStatsProjection` type +
  `ProjectSessionStats()` function, ported from DSH's session-stats projection.
  Tracks whole-log conversation figures: turns, steps, LLM wall time, tool wall
  time, first-token latency (TTFT), decode wall time, and output token counts.
  Folds over the event spine using step/start-end-bracketing and event At
  timestamps.
- `internal/eventlog/plan_mode_test.go` — tests for PlanMode round-trip, Known(),
  wire decode, and SessionStats projection (single-turn/step and empty log).
- `docs/plans/dsh-harness-port-plan.md` — Phase 10 section appended documenting
  all deliverables.

## Phase 11 — DSH parity field enhancements + permission projection

Delivered on this branch on top of Phase 10:

- `internal/eventlog/boundary.go` — DSH parity field enhancements to approval
  fact structs (all optional/omitempty for backward compatibility):
  - `ApprovalAskedFact` — added `ID` (ApprovalRequestId pairing ask↔decide),
    `CallID` (exact tool call id).
  - `ApprovalDecidedFact` — added `ID` (pairs with asked), `Outcome`
    ("allowed-once"|"rejected"|"cancelled"|"unavailable").
  - `ApprovalPolicyFact` — added `PresetName`, `Policy` ("ask"|"never"),
    `Source` ("delegation" for seeded child override).
- `internal/eventlog/lifecycle.go` — DSH parity field enhancements:
  - `PermissionPresetFact` — added `PresetName` matching DSH's `{ preset: string }`.
  - `CommandRunFact` — added `CommandID`, `Name`, `Args`, `Source`
    matching DSH's `command/run` payload.
  - `CommandDoneFact` — added `CommandID`, `Kind` ("success"|"error"),
    `Text`, `SourceEventSeq` matching DSH's `command/done` payload.
- `internal/eventlog/projection.go` — `ProjectPermissions()`:
  Folds from `permission/preset`, `sandbox/mode`, and `approval/policy` events
  into a `PermissionSelect` view (options list + effective current value),
  matching DSH's permissions projection. `PresetOption` type added.
- `internal/engine/work_mode.go` — `SetWorkMode()` now emits `plan.mode` journal
  events when the mode actually transitions (plan↔act), matching DSH's
  `plan/mode` last-write-wins folding.
- `internal/engine/agent_session_tool.go` — `spawnSubAgentRequest` upgraded to
  use `AppendSubagentDescriptorFull` with DSH v2 parity fields (Mode, Provider,
  Label, AgentProvider, AgentModel).
- `internal/eventlog/plan_mode_test.go` — added tests for
  `ProjectPermissions` covering preset/policy/sandbox/default/empty cases.

## Phase 12 — Relational invariants + write-behind batching + preparations cache

Delivered on this branch on top of Phase 11:

- `internal/eventlog/invariants.go` — `ValidateRelations()`: DSH-compatible
  relational invariant checking (ported from dsh-session/invariant.ts):
  - Turn/step nesting: turn/start, step/start, step/end must match open turn/step
  - Tool call↔result pairing: tool/result with surfaceOp: append must have prior
    tool/call with matching ID (unless fail-closed TOOL_NOT_STARTED)
  - Core execution events (todo/write, request/header, request/context) must
    be turn-enclosed
  - Seq must strictly increase
  - Surface replacement exempt from pairing check
- `internal/eventlog/event.go` — added `Turn`/`Step` fields to `Message` and
  `ToolCallPayload`/`ToolResultPayload` for relational invariant checks
- `internal/eventlog/invariants_test.go` — 7 tests for relational validation
- `internal/session/write_behind.go` — Go-native port of DSH's
  `SessionWriteBehind` class (write-behind.ts):
  - Bounded per-session write batching with fixed timer deadline
  - Background write with failure retention (re-queues on error)
  - Quiescence barrier: Flush() drains all pending events synchronously
  - Concurrent flushes join the same barrier (no double-flush)
  - CancelAutomaticWait() cancels the batching timer
  - ReportBackgroundFailure callback for failure observation
- `internal/session/write_behind_test.go` — 6 tests for write-behind controller
- `internal/session/preparations.go` — Go-native port of DSH's
  `SessionPreparations` class (preparations.ts):
  - Bounded LRU cache of prepared session sources with load-sharing
  - Phase-based state machine: loading/ready/committing/reserved
  - Exclusive reservation: Reserve() transitions through committing→reserved
  - Release/Discard/Attach/TakeReady/Invalidate/DiscardReady methods
  - AssertWritable for phase-based write admission control
  - LRU eviction at capacity
- `internal/session/preparations_test.go` — 9 tests for preparations cache
- `internal/session/session.go` — `ValidateRelations` wired into load/recovery
  paths as best-effort warning
- `internal/session/branch.go` — `ValidateRelations` wired into `RepairJournal`
- `internal/engine/persistence_service.go` — `writeBehind` field added
- `internal/engine/journal.go` — `SetWriteBehind()`/`WriteBehind()`/`FlushWriteBehind()`
  methods + imports for session package

## Phase 13 — ACP content admission

Ported from DSH `packages/acp/acp/src/content.ts` so the ACP server admits
real inline multimodal content:

- `internal/acp/content.go` — Go-native admission module:
  - `AcpContentBlock` (wire shape: `type`/`text`/`mimeType`/`data`/`name`/`uri`),
    `ContentBlock` (durable core content with `*attachment.Ref`), and
    `ContentError{Kind,Msg,Err}` with `FailureInvalid`/`FailureInternal`.
  - `AdmitAcpPrompt(ctx, store, prompt, imageEnabled, signal)` — validates all
    blocks first (rejects audio/resource/unknown as invalid; rejects images
    when `imageEnabled` is false, base64 is non-canonical, or mimeType is
    non-raster), persists the image batch atomically via `store.SaveImages`,
    reconstructs ordered content, and rejects empty prompts.
  - `decodeImage`/`imageMediaType`/`resourceLinkText`/`checkAborted`,
    `SupportsAcpImagePrompts`, `AssistantBlockToAcp`.
- `internal/acp/server.go` — wired into the server:
  - `Server.store` + `imageCapable` fields; `SetAttachmentStore(store, modelSupportsImage)`.
  - `initialize` advertises `promptCapabilities.image` truthfully from `imageCapable`.
  - `handlePrompt` runs `AdmitAcpPrompt` before queuing any user message (so
    no late message races a persisted image), maps `failure-invalid`→invalid-params
    / `failure-internal`→internal-error, coalesces consecutive text into one
    `AddUser`, and attaches images via `session.AddUserWithAttachment`.
- `internal/acp/content_test.go` — 13 admission tests + 3 server integration
  tests (capability advertisement, inline image admission end-to-end, and
  rejection of unadvertised images).

## Latest DSH clone check

Cloned the latest `deepseek-ai/deepseek-harness` (rc.7, Aug 17) and compared against
the older clone (rc.7, Aug 13). Results: no new event types, no new invariants,
no persistence protocol changes. 7 new files in the latest clone — all client-side
UI additions (Safari support, outside pointer handling) or ACP content admission
(`packages/acp/acp/src/content.ts`). The eventlog and session format are stable.

## Gates

Each phase: `make ci` + `hawk verify`; no direct commits to `main`.
