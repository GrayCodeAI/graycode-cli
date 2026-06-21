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

## Initial package layout

```text
hawk-core-contracts/
├── types/
├── review/
├── verify/
├── events/
├── policy/
├── tools/
├── engines/
├── sessions/
└── README.md
```

## Initial contracts to move

### Findings
- `Severity`
- `Finding`
- `ReviewFinding`
- `VerificationFinding`

### Events
- `TraceEvent`
- `SessionEvent`
- `ToolEvent`
- `VerificationEvent`

### Policy
- `PolicyDecision`
- `ApprovalRequirement`
- `ExecutionBoundary`

### Tools
- `ToolCall`
- `ToolResult`
- `ToolError`

### Engines
- `EngineRequest`
- `EngineResponse`
- engine-specific input/output envelopes

### Sessions
- `SessionID`
- `SessionState`
- `SessionSummary`

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
- Hawk session persistence has started migrating to provider-neutral tool contracts
- Hawk review storage and inspect/review bridge paths now consume neutral review/verify contracts
- Hawk transport config/provider seams are now Hawk-owned and adapted to `eyrie/client` at the edge

## Versioning rule

Keep `hawk-core-contracts` conservative:

- additive changes first
- avoid breaking changes when possible
- treat contracts like a public API, even if only used internally at first
