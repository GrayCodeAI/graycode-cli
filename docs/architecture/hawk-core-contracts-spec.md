# Hawk Core Contracts Spec

## Purpose

`hawk-core-contracts` is the shared language spoken by Hawk and its engines.

It exists to remove cross-repo dependency on Hawk internals and to prevent engines from importing each other.

## Scope

Allowed contents:

- shared enums
- shared structs
- request/response contracts
- event schemas
- finding/result models
- engine interfaces
- policy decision models

Not allowed:

- CLI code
- provider code
- runtime logic
- storage implementations
- engine implementations
- Hawk application internals

## Package layout

Implemented today:

```text
hawk-core-contracts/
├── types/    # findings + severity vocabulary
├── review/   # code-review result contracts
├── verify/   # verification result contracts
├── events/   # trace/tool event schemas
├── policy/   # permission/guardian decision contracts
├── tools/    # provider-neutral tool call/result
├── graph/    # portable nodes, edges, lifecycle events, scope, provenance
└── README.md
```

Planned, not yet implemented (do not document as present until the package exists):

```text
├── engines/   # engine request/response envelopes
└── sessions/  # session id/state/summary contracts
```

## Implemented contracts

These are the types that actually live in `hawk-core-contracts` today. Keep this
list in sync with the code (it is the inventory the dependency rules assume).

### `types/`
- `Severity`, `AuditSeverity`, `TokenSeverity`
- `Finding`, `FindingSlice`, `FindingSummary`
- helpers: `ParseSeverity`, `FindingFromSight`, `FindingFromInspect`

### `review/`
- `Result`, `Finding`, `InlineComment`, `Stats`, `ConfidenceBreakdown`

### `verify/`
- `Report`, `Finding`, `Stats`

### `events/`
- `TraceEvent`, `ToolEvent`, `UsageInfo`

### `policy/`
- `GuardianDecision`, `PermissionRequest`, `PermissionVerdict`, `Risk`
- helpers: `Allow`, `Deny`, `RequireApproval`, `ParseRisk`

### `tools/`
- `ToolCall`, `ToolResult`

### `graph/`
- `Node`, `Edge`, `Event`, `Ref`
- `NodeKind`, `EdgeKind`, `EventType`
- `Scope`, `Provenance`, `ArtifactRef`

### Planned (not yet in the module)
- engines: `EngineRequest`, `EngineResponse`, engine-specific envelopes
- sessions: `SessionID`, `SessionState`, `SessionSummary`

## Migration order

1. inventory cross-repo shared types in `hawk/shared` and `hawk/internal/types`
2. move stable shared types into contracts
3. update `sight`
4. update `inspect`
5. update other repos that rely on Hawk-exported shared types
6. leave product-only types inside Hawk

## Current status

- severity and finding contracts are live in `hawk-core-contracts`
- review result contracts now exist in `hawk-core-contracts/review`
- verification result contracts now exist in `hawk-core-contracts/verify`
- tool contracts now exist in `hawk-core-contracts/tools`
- event contracts now exist in `hawk-core-contracts/events`
- policy contracts now exist in `hawk-core-contracts/policy`
- portable graph contracts now exist in `hawk-core-contracts/graph`
- Hawk session persistence has started migrating to provider-neutral tool contracts
- Hawk review storage and inspect/review bridge paths now consume neutral review/verify contracts
- Hawk runtime conversation DTOs and the `ChatClient` port are Hawk-owned and
  translated to the stable `eyrie/engine` contract at the integration edge

## Versioning rule

Keep `hawk-core-contracts` conservative:

- additive changes first
- avoid breaking changes when possible
- treat contracts like a public API, even if only used internally at first
