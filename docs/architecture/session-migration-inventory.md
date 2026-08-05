# Session Migration Inventory

**Status:** Phase 2 inventory
**Date:** 2026-08-04
**Branch:** `chore/architecture-phase0-baseline`

This inventory is the migration gate for `internal/engine.Session`. The
Session refactor is intentionally high risk because the type is used by the
agent loop, compaction, command entry points, daemon construction, and
multi-agent workers.

## Impact analysis

GitNexus impact analysis was run upstream against the current indexed commit.

| Symbol | Direct callers | Impacted symbols | Processes | Modules | Risk |
|---|---:|---:|---:|---:|---|
| `Session` | 1 | 9 | 1 | 3 | HIGH |
| `NewSessionWithClient` | 3 | 20 | 4 | 3 | HIGH |
| `Session.Persistence()` | 23 | 34 | not summarized | primarily Engine | HIGH |

The affected named execution flows include:

- `ReadOnlyValidationWorker`
- `runExec`
- `runMission`
- `runDaemonStart`

The GitNexus index did not resolve a symbol named `AgentLoop`; the agent-loop
implementation is represented by other stream functions and must be mapped by
file and context before any stream symbol is edited.

## Caller groups

### Construction

`NewSessionWithClient` is called by:

- `internal/engine/session_factory.go`
- `internal/multiagent/worker.go`
- daemon and benchmark test factories
- resilience, compaction, and stream integration tests
- `Session.SubSession`

The production construction path is therefore the factory plus the sub-session
path. Tests also construct sessions directly and must be migrated or explicitly
retained as test-only fixtures before compatibility fields are removed.

### Persistence access

`Session.Persistence()` is used by:

- `internal/engine/stream.go`
- `internal/engine/engine.go`
- `internal/engine/compact*.go`
- `internal/engine/context_governor.go`
- `internal/engine/context_compaction.go`
- session message/context methods in `session.go`
- council and lifecycle/tool integration paths
- session, compaction, resilience, and integration tests

The dominant access pattern is repeated read-modify-write through
`RawMessages()`, `SetRawMessages()`, `System()`, and compaction metadata. This
is a service API migration, not a simple field rename.

### Direct struct literals

Several tests use `Session{...}` directly. These fixtures are the reason the
current implementation retains lazy service materialization. They must be
classified as either:

1. constructor tests that should use `NewSessionWithClient`;
2. focused service tests that should instantiate the service directly; or
3. intentional low-level fixtures with an explicit test-only builder.

No production compatibility path should be removed until this classification
is complete.

## Migration sequence

The first bounded slice is complete: transcript/system state, token
accounting, token-estimate cache, and checkpoint-manager state now have one
owner in `PersistenceService`. `persistID` remains dual-written pending the
graph/journal migration slice. Zero-value lazy service materialization remains
as a compatibility seam until direct construction fixtures are classified. A
second slice is complete: LLM client/provider/model identity now has one owner
in `ChatService`, with synchronized access and reattachment.

1. Freeze new direct reads of legacy Session fields.
2. Add or complete named service methods for each remaining access pattern.
3. Migrate one caller group at a time, starting with session accessors and
   low-risk tests.
4. Migrate compaction and context governance as separate changes because they
   mutate message state and have the largest persistence fan-out.
5. Migrate stream orchestration only after persistence and context contracts
   are stable.
6. Replace direct struct literals with test builders.
7. Remove lazy service materialization and obsolete legacy fields.
8. Run impact analysis and the full verification suite after every step.

## Safety gates

- No broad find-and-replace on Session fields.
- Run `impact` upstream before modifying each function or method.
- Warn before proceeding on HIGH or CRITICAL impact.
- Preserve behavior with focused tests before removing compatibility paths.
- Run `make boundaries`, `go test ./internal/engine/...`, and the full suite
  after each migration group.
- Run `detect_changes --scope compare --base-ref main` before committing.

## Exit criteria

Phase 2 is complete only when:

- service state is the only authoritative runtime state;
- `Session` no longer contains duplicate legacy state;
- no production caller depends on lazy `Persistence()` fallback behavior;
- all direct struct-literal fixtures use an intentional test builder;
- session, compaction, recovery, and multi-agent tests pass;
- the final impact report shows the expected reduced fan-out.
