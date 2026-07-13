# Hawk Contract Migration Inventory

## Goal

This document captures the current shared-type coupling that should be moved into `hawk-core-contracts`.

## Current cross-repo export surface

### `hawk/shared/types`

Status: removed

The legacy Hawk-owned shared type shim has been deleted. Shared severity and
finding contracts now live only in `hawk-core-contracts/types`.

## Current external consumers

### `sight`

Migration status: completed

Current usage:

- severity aliasing
- review concern severity typing

### `inspect`

Migration status: completed

Current usage:

- severity aliasing
- check severity
- report formatting / finding output

### `hawk` docs and metadata

Current references:

- `README.md`
- `AGENTS.md`
- architecture docs

These will need copy updates once the migration is completed.

## Current Hawk-internal types

### `hawk/internal/types`

Files:

- `internal/types/client.go`
- `internal/types/settings.go`
- `internal/types/severity.go`

Assessment:

- `internal/types/severity.go` now re-exports `hawk-core-contracts/types`
- `internal/types/client.go` contains Hawk-owned conversation/runtime DTOs and
  the small provider port needed by product integrations; it has no Eyrie imports
- `internal/types/settings.go` is Hawk config-specific and should remain Hawk-internal

## Tool contract migration

### Historical source shape

Before the engine-boundary migration, runtime source types included
lower-level provider tool-call and tool-result DTOs.

### New neutral contract

Added:

- `hawk-core-contracts/tools.ToolCall`
- `hawk-core-contracts/tools.ToolResult`

### First migration boundary

Hawk session persistence now uses neutral tool contracts instead of persisting
lower-level provider types directly.

### Remaining migration

- Hawk runtime now owns `internal/types.EyrieMessage`
- Hawk runtime now owns tool call/result, response, usage, and stream DTOs in `internal/types`
- Hawk runtime now owns chat options, response format, continuation config, tool choice, and tool definition DTOs in `internal/types`
- Hawk runtime now owns the provider seam via `internal/types.ChatProvider`
- Hawk's `ChatClient` port is implemented by `internal/engine` using only
  `eyrie/engine`; no production package imports a lower Eyrie package
- future work should move trace/event/policy layers to consume neutral tool contracts where appropriate

## Review and verification contract migration

### New shared contracts

Added:

- `hawk-core-contracts/review.Finding`
- `hawk-core-contracts/review.InlineComment`
- `hawk-core-contracts/review.Stats`
- `hawk-core-contracts/review.Result`
- `hawk-core-contracts/verify.Finding`
- `hawk-core-contracts/verify.Stats`
- `hawk-core-contracts/verify.Report`

### Current adoption

- `sight` now exposes adapters from its public result types into `hawk-core-contracts/review`
- `inspect` now exposes adapters from its public report types into `hawk-core-contracts/verify`
- Hawk review persistence now stores neutral review findings instead of `sight`-owned findings
- Hawk inspect/review bridge paths now return neutral review/verification contracts for product-facing integration

### Remaining migration

- `sight.Result` still carries sight-specific SAST fusion details outside the shared contract
- `inspect.Report` remains the public engine-local type and converts at the boundary
- review status lifecycle enums still live in Hawk because they are product workflow state, not cross-repo contracts

## Event contract migration

### New shared contracts

Added:

- `hawk-core-contracts/events.ToolEvent`
- `hawk-core-contracts/events.TraceEvent`
- `hawk-core-contracts/events.UsageInfo`

### Current adoption

- `internal/hooks/audit.ToolEvent` now aliases the shared contract
- `internal/observability/oteltrace.TraceEvent` now aliases the shared contract

### Remaining migration

- broader session/timeline/workflow event types are still Hawk-internal
- policy and verification event schemas can move next as separate contracts

## Policy contract migration

### New shared contracts

Added:

- `hawk-core-contracts/policy.Risk`
- `hawk-core-contracts/policy.PermissionVerdict`
- `hawk-core-contracts/policy.GuardianDecision`
- `hawk-core-contracts/policy.PermissionRequest`

### Current adoption

- `internal/permissions.PermissionVerdict` now aliases the shared contract
- `internal/permissions.GuardianDecision` now aliases the shared contract
- `internal/engine/safety.PermissionRequest` now embeds the shared request contract

### Remaining migration

- sandbox-specific policy manager types remain Hawk-internal
- approval gate categories remain Hawk-internal

## Migration order

### Step 1
Scaffold `hawk-core-contracts` with `types/` for severity and findings.

### Step 2
Update `sight` to import `github.com/GrayCodeAI/hawk-core-contracts/types`.

### Step 3
Update `inspect` to import `github.com/GrayCodeAI/hawk-core-contracts/types`.

### Step 4
Update `hawk/internal/types/severity.go` to re-export from `hawk-core-contracts/types`. Completed.

### Step 5
Update docs that currently describe `hawk/shared/types` as the cross-repo API.

Status: completed.

### Step 6
Remove `hawk/shared/types` after local migration completes. Completed.

Current status:

- local ecosystem migration is complete
- Hawk no longer ships the `hawk/shared/types` package
