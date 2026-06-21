# Hawk Architecture v1 — Definition of Done

This document defines **realistic v1 complete** for the Hawk ecosystem architecture.

It is the shipping bar for the contracts-and-boundaries refactor. It is **not** a promise that every future package in `hawk-core-contracts-spec.md` exists, or that every engine is wired through shared contracts on every runtime path.

## What v1 means

v1 is done when the ecosystem has:

- one product surface (`hawk`)
- six peer support engines with no sibling imports
- shared vocabulary only where cross-repo pain is real
- automated guards so the old coupling cannot return

v1 does **not** require a single unified contract layer for runtime, persistence, trace, and orchestration.

## v1 checklist

### Product and dependency graph

- [ ] `hawk` is the only primary end-user product in the ecosystem
- [ ] Hawk coordinates engines; engines do not import each other
- [ ] SDKs and community skills consume Hawk public surfaces, not engines directly
- [ ] `graycode-core` and other company/platform repos stay outside Hawk runtime dependencies

### Forbidden edges stay forbidden

- [ ] no support repo imports `hawk/internal/*`
- [ ] no support repo imports removed `hawk/shared/types`
- [ ] no SDK/skills repo references support engines as primary dependencies
- [ ] Hawk production code imports `eyrie/client` only at the transport adapter edge

### `hawk-core-contracts` (implemented packages only)

These packages are in scope for v1:

- [ ] `types/` — severity and finding vocabulary
- [ ] `review/` — neutral review results
- [ ] `verify/` — neutral verification reports
- [ ] `tools/` — persisted tool call/result contracts
- [ ] `events/` — normalized audit/trace event subset used by Hawk
- [ ] `policy/` — permission and guardian decision contracts

Adoption bar:

- [ ] `sight` and `inspect` import contracts for shared severity/findings and expose boundary adapters
- [x] `tok/types` compatibility shim removed from the local ecosystem
- [ ] `eyrie`, `yaad`, and `trace` remain contract-free unless they gain a true cross-repo type

### Hawk integration seams

- [ ] session persistence uses `hawk-core-contracts/tools`, not `eyrie/client` tool types
- [ ] review persistence and inspect/review bridge paths use neutral `review` / `verify` contracts
- [ ] Hawk owns runtime transport DTOs in `internal/types` and adapts to `eyrie/client` at the edge
- [ ] `hawk trace ...` remains a Hawk-mounted subcommand, not a competing product surface

### Enforcement

- [ ] Hawk CI runs ecosystem, shared-types, eyrie-client, and peer-coupling guards
- [ ] each support repo runs `check-ecosystem-boundaries.sh` in CI
- [ ] Go SDK runs consumer boundary guard in CI
- [ ] Python SDK and community skills run consumer boundary guards in CI
- [ ] architecture docs do not describe removed or planned packages as currently shipped
- [ ] lefthook strips `Co-authored-by:` trailers so commits list only the human author

### Ship state

- [ ] open architecture PRs for engines, consumers, and Hawk integration are merged to `main`
- [ ] published module versions used by Hawk match the merged contract changes

## Explicit non-goals for v1

Do not block v1 on any of the following:

- `hawk-core-contracts/sessions` or `hawk-core-contracts/engines`
- moving every Hawk internal event struct into `hawk-core-contracts/events`
- moving trace timeline/event models out of `trace`
- unifying runtime `internal/types` DTOs and persisted contracts into one type
- forcing every engine into the same integration depth (library vs subcommand vs service)
- deep static import analysis for Python SDK or markdown-only skills repos

## Governance after v1

Add a new `hawk-core-contracts` package only when **all** of these are true:

1. at least two repos need the same stable type or envelope
2. the type is vocabulary or DTO, not runtime logic
3. the owning repo cannot keep the type local without recreating cross-repo coupling
4. the addition is documented in `hawk-core-contracts-spec.md` before code lands

Remove compatibility shims only when:

1. grep/guards show zero remaining importers
2. the removal is called out in the repo CHANGELOG or migration note

## Verify locally

From `hawk`:

```bash
make ecosystem-guard contracts-guard eyrie-client-guard peer-guard
go test ./internal/testaudit/... -count=1
go test ./... -count=1
```

From each support repo:

```bash
bash ./scripts/check-ecosystem-boundaries.sh
go test ./... -count=1
```

## Related docs

- `hawk-product-architecture.md` — target shape and phases
- `hawk-dependency-rules.md` — allowed and forbidden edges
- `hawk-core-contracts-spec.md` — contract inventory and planned packages
- `../plans/hawk-contracts-migration-backlog.md` — migration history and follow-ups
