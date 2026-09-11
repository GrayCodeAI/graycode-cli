# Plan: Hawk Contracts Migration Backlog

> Status: locally complete
> Scope: graycode-eco ecosystem architecture cleanup after introducing `eagle`
> Goal: keep `hawk` as the product while moving stable cross-repo contracts out of Hawk internals

External follow-up still outside the scope of this local workspace audit:

- confirm upstream branches contain the final architecture commits
- confirm published module tags/releases match the merged contract changes

## Done

These items are already completed in the current workspace.

### Contracts repo scaffold
- created `eagle`
- added `go.mod`
- added package docs

### Shared type migration
- added `eagle/types`
- moved severity and finding definitions into contracts
- migrated `kestrel` and `merlin` to import contracts
- switched Hawk `internal/types/severity.go` to re-export from contracts
- removed `hawk/shared/types` after local ecosystem migration
- removed the duplicate severity/finding definitions in `shrike/types` (shrike was the
  original shared-types host)
- removed the `shrike/types` compatibility shim after verifying no in-workspace
  importers remained

### Tool contract migration
- added `eagle/tools`
- switched Hawk session persistence to provider-neutral tool contracts
- added runtime/session conversion helpers
- added `eagle/tools/tool_test.go`
- centralized message slice conversion in `hawk/internal/session`
- removed direct lower-level provider message reconstruction from Hawk
  cmd/session restore paths
- replaced the provider-owned message alias with a Hawk-owned runtime DTO
- added explicit Hawk-owned runtime DTOs; the final boundary translates them
  through `eyrie/engine`

### Event contract migration
- added `eagle/events`
- switched normalized audit `ToolEvent` to shared contract
- switched Langfuse swift event model to shared contract
- added `eagle/events/events_test.go`

### Policy contract migration
- added `eagle/policy`
- switched permission verdict and guardian decision to shared contracts
- switched engine permission request to embed the shared request contract
- added `eagle/policy/policy_test.go`

### Governance
- added `check-shared-types-imports.sh`
- added `check-ecosystem-boundaries.sh`
- added `check-eyrie-client-imports.sh`
- wired both guards into `Makefile` and CI
- wired the Eyrie engine-boundary guards into `Makefile` and CI
- added a legacy import guard so the removed `shared/types` path cannot return
- extended the ecosystem boundary guard to scan sibling engine repos when present locally
- updated docs across Hawk, kestrel, merlin, and external workspace copies
- added standalone boundary guards in `kestrel` and `merlin`
- added standalone boundary guards in `shrike`, `eyrie`, `harrier`, and `swift`
- updated support repo READMEs with ecosystem boundary rules

### Review and verification contract migration
- added `eagle/review`
- added `eagle/verify`
- added shared review/verification contract tests
- added kestrel -> review contract adapters
- added merlin -> verify contract adapters
- switched Hawk review persistence to neutral review contracts
- switched Hawk review/merlin bridge paths to return neutral review/verify contracts

## Remaining external follow-up

These are the highest-value follow-up tasks that this local workspace cannot
prove automatically.

### 1. Confirm upstream/default-branch convergence

Local state:
- implemented and verified in this workspace

Still external:
- confirm the same commits are merged on the intended upstream default branches

### 2. Confirm release/publication convergence

Local state:
- Hawk local integration snapshot points at architecture-aligned support-repo revisions

Still external:
- confirm released module versions used by Hawk match the merged contract changes

### 3. Remove Hawk production dependency on lower Eyrie packages
Current state:
- session persistence uses neutral tool contracts
- Hawk now owns the runtime message DTO in `internal/types.EyrieMessage`
- Hawk now owns runtime tool call/result DTOs in `internal/types`
- Hawk now owns runtime response/usage/stream DTOs in `internal/types`
- Hawk now owns runtime chat options, response format, tool choice, continuation config, and tool definition DTOs in `internal/types`
- Hawk now owns the transport-provider seam in `internal/types.ChatProvider`
- Hawk session, review, setup, catalog, diagnostics, and custom-provider paths
  now enter through `eyrie/engine`
- production imports of every lower Eyrie package are zero
- cmd/session restore paths now go through centralized `session.ToRuntimeMessages` and `session.FromRuntimeMessages`

Decision:
- completed: Hawk owns the product DTO/port seam and Eyrie owns the engine,
  provider, credential, catalog, routing, resilience, and normalization layers
- keep the zero-exception boundary enforced; lower packages are test-fixture-only

## Later

These are useful, but should be done only if they solve real cross-repo pain.

### 3. Add session contracts
Potential package:
- `eagle/sessions`

Candidates:
- `SessionID`
- `SessionSummary`
- portable persisted message DTOs

Do this only if another repo truly needs them.

### 4. Decide whether more review/verification metadata should move into contracts
Current state:
- normalized review result contracts now live in `eagle/review`
- normalized verification report contracts now live in `eagle/verify`
- Hawk consumes neutral review/verification contracts at persistence and bridge boundaries

Possible later additions:
- review lifecycle status enums if another repo needs them
- SAST fusion metadata if it needs to cross repo boundaries
- richer verification provenance fields beyond the current shared report

### 5. Add swift/timeline event families beyond the current normalized contracts
Current `events` package is intentionally minimal.

Possible later additions:
- session lifecycle events
- verification events
- workflow events
- tool execution lifecycle events

Do not move every internal event shape by default.

### 6. Remove `hawk/shared/types`
Completed for the local ecosystem.

Current status:

- Hawk no longer ships `hawk/shared/types`
- local import guards prevent the old path from returning

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
- completed: removed all lower-level Eyrie imports from Hawk production code and
  replaced the compatibility allowlist with a zero-exception guard

### PR 4
- completed: added neutral review/verification result contracts and wired Hawk bridge/persistence edges

### PR 5
- completed: removed `hawk/shared/types` from the local ecosystem and kept a legacy import guard

## Success criteria

The migration is in a good long-term state when:

- `eagle` is the only source of truth for shared contracts
- support repos do not import `hawk/internal/*`
- support repos do not import `hawk/shared/types`
- removed compatibility shims do not return
- CI prevents regressions
- new shared contracts are added deliberately, not by habit
