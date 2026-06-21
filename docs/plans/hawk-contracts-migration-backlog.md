# Plan: Hawk Contracts Migration Backlog

> Status: active
> Scope: Hawk ecosystem architecture cleanup after introducing `hawk-core-contracts`
> Goal: keep `hawk` as the product while moving stable cross-repo contracts out of Hawk internals

## Done

These items are already completed in the current workspace.

### Contracts repo scaffold
- created `hawk-core-contracts`
- added `go.mod`
- added package docs

### Shared type migration
- added `hawk-core-contracts/types`
- moved severity and finding definitions into contracts
- migrated `sight` and `inspect` to import contracts
- switched Hawk `internal/types/severity.go` to re-export from contracts
- converted `hawk/shared/types` into a compatibility shim
- removed the duplicate severity/finding definitions in `tok/types` (tok was the
  original shared-types host); `tok/types` now re-exports
  `hawk-core-contracts/types` as a deprecated compatibility shim

### Tool contract migration
- added `hawk-core-contracts/tools`
- switched Hawk session persistence to provider-neutral tool contracts
- added runtime/session conversion helpers
- added `hawk-core-contracts/tools/tool_test.go`
- centralized message slice conversion in `hawk/internal/session`
- removed direct `eyrie/client` message reconstruction from Hawk cmd/session restore paths
- replaced the `internal/types.EyrieMessage` alias with a Hawk-owned runtime DTO
- added explicit `internal/types` adapters between Hawk runtime messages and `eyrie/client`

### Event contract migration
- added `hawk-core-contracts/events`
- switched normalized audit `ToolEvent` to shared contract
- switched Langfuse trace event model to shared contract
- added `hawk-core-contracts/events/events_test.go`

### Policy contract migration
- added `hawk-core-contracts/policy`
- switched permission verdict and guardian decision to shared contracts
- switched engine permission request to embed the shared request contract
- added `hawk-core-contracts/policy/policy_test.go`

### Governance
- added `check-shared-types-imports.sh`
- added `check-ecosystem-boundaries.sh`
- added `check-eyrie-client-imports.sh`
- wired both guards into `Makefile` and CI
- wired the `eyrie/client` boundary guard into `Makefile` and CI
- tightened `shared/types` deprecation comments on the compatibility package itself
- extended the ecosystem boundary guard to scan sibling engine repos when present locally
- updated docs across Hawk, sight, inspect, and external workspace copies
- added standalone boundary guards in `sight` and `inspect`
- added standalone boundary guards in `tok`, `eyrie`, `yaad`, and `trace`
- updated support repo READMEs with ecosystem boundary rules

### Review and verification contract migration
- added `hawk-core-contracts/review`
- added `hawk-core-contracts/verify`
- added shared review/verification contract tests
- added sight -> review contract adapters
- added inspect -> verify contract adapters
- switched Hawk review persistence to neutral review contracts
- switched Hawk review/inspect bridge paths to return neutral review/verify contracts

## Next

These are the highest-value follow-up tasks.

### 1. Extend standalone import guards to the remaining support repos
Done:
- `sight`
- `inspect`
- `tok`
- `eyrie`
- `yaad`
- `trace`

### 2. Decide whether Hawk session/API message DTOs should fully stop using `eyrie/client` types
Current state:
- session persistence uses neutral tool contracts
- Hawk now owns the runtime message DTO in `internal/types.EyrieMessage`
- Hawk now owns runtime tool call/result DTOs in `internal/types`
- Hawk now owns runtime response/usage/stream DTOs in `internal/types`
- Hawk now owns runtime chat options, response format, tool choice, continuation config, and tool definition DTOs in `internal/types`
- Hawk now owns transport client construction config in `internal/types.ClientConfig`
- Hawk now owns the transport-provider seam in `internal/types.ChatProvider`
- `eyrie/client` is now adapted only at the `internal/types` edge and a few provider-registry helper paths
- cmd/session restore paths now go through centralized `session.ToRuntimeMessages` and `session.FromRuntimeMessages`

Decision:
- completed: Hawk now owns the transport config/provider seam
- keep direct `eyrie/client` usage limited to adapter implementations and provider-registry integration only

This should be a deliberate decision, not drift.

## Later

These are useful, but should be done only if they solve real cross-repo pain.

### 3. Add session contracts
Potential package:
- `hawk-core-contracts/sessions`

Candidates:
- `SessionID`
- `SessionSummary`
- portable persisted message DTOs

Do this only if another repo truly needs them.

### 4. Decide whether more review/verification metadata should move into contracts
Current state:
- normalized review result contracts now live in `hawk-core-contracts/review`
- normalized verification report contracts now live in `hawk-core-contracts/verify`
- Hawk consumes neutral review/verification contracts at persistence and bridge boundaries

Possible later additions:
- review lifecycle status enums if another repo needs them
- SAST fusion metadata if it needs to cross repo boundaries
- richer verification provenance fields beyond the current shared report

### 5. Add trace/timeline event families beyond the current normalized contracts
Current `events` package is intentionally minimal.

Possible later additions:
- session lifecycle events
- verification events
- workflow events
- tool execution lifecycle events

Do not move every internal event shape by default.

### 6. Remove `hawk/shared/types`
Only after:
- no external repos import it
- no compatibility reason remains
- release/migration notes are ready
- deprecation warnings have been visible long enough for downstream users

Until then it stays as a compatibility shim.

Current status:

- local workspace is ready for retirement
- external downstream compatibility is not yet proven from this repo alone
- removal should happen only as an explicit breaking-change release step

## Non-goals

Do not do these without a separate decision:

- move provider runtime types into contracts
- move Hawk orchestration logic into contracts
- move sandbox manager internals into contracts
- move every event struct in Hawk into contracts
- force Lark/Gitant architecture into Hawk contracts

## Recommended PR order

### PR 1
- completed: moved chat options/request DTOs behind Hawk-owned runtime adapters

### PR 2
- completed: moved provider/config interfaces behind Hawk-owned transport adapters

### PR 3
- continue reducing any remaining non-adapter direct `eyrie/client` imports in Hawk where it improves clarity

### PR 4
- completed: added neutral review/verification result contracts and wired Hawk bridge/persistence edges

### PR 5
- remove `hawk/shared/types` only when safe

## Success criteria

The migration is in a good long-term state when:

- `hawk-core-contracts` is the only source of truth for shared contracts
- support repos do not import `hawk/internal/*`
- support repos do not import `hawk/shared/types`
- compatibility shims are temporary and clearly documented
- CI prevents regressions
- new shared contracts are added deliberately, not by habit
