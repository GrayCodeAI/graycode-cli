# hawk-core-contracts

Shared contracts for the Hawk ecosystem.

This repo exists to hold stable cross-repo definitions used by:

- `hawk`
- `eyrie`
- `yaad`
- `tok`
- `trace`
- `sight`
- `inspect`
- Hawk SDKs and extension surfaces where needed

## Scope

Allowed here:

- shared enums
- shared structs
- event models
- finding/result models
- engine request/response contracts
- policy and tool contracts

Not allowed here:

- CLI code
- provider implementations
- runtime logic
- storage implementations
- product orchestration

## Initial migration target

The first migration target is the current Hawk package:

- `github.com/GrayCodeAI/hawk/shared/types`

That package currently exports severity and finding models for cross-repo use. Those definitions should move here so support repos stop depending on the Hawk product repo for shared contracts.

## Package map

- `types/` - severity, findings, and shared result vocabulary
- `tools/` - provider-neutral tool call and tool result contracts
- `events/` - normalized tool and trace event contracts
- `policy/` - risk, permission verdict, guardian decision, approval request contracts
- `review/` - neutral review findings, comments, stats, and result contracts
- `verify/` - neutral verification findings, stats, and report contracts

## Current status

Completed:

1. shared finding and severity definitions moved here
2. `sight` and `inspect` migrated to import this repo
3. Hawk docs and READMEs updated
4. tool, event, and policy contracts added
5. review and verification result contracts added

## Governance rules

- keep this repo implementation-free
- prefer additive changes
- avoid product-specific runtime assumptions
- do not move Hawk orchestration code here
- if a type is only used inside one repo, it should stay in that repo
