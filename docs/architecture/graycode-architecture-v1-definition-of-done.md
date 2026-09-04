# Graycode Architecture v1 — Definition of Done

This document defines **realistic v1 complete** for the Graycode main CLI and its
connected ecosystem architecture. `graycode-eco` is only the local parent
folder; it is not a repository or product.

It is the shipping bar for the contracts-and-boundaries refactor. It is **not** a promise that every future package in `eagle-spec.md` exists, or that every engine is wired through shared contracts on every runtime path.

## What v1 means

v1 is done when the ecosystem has:

- one product surface (`graycode`)
- six peer support engines with no sibling imports
- shared vocabulary only where cross-repo pain is real
- automated guards so the old coupling cannot return

v1 does **not** require a single unified contract layer for runtime, persistence, swift, and orchestration.

## v1 checklist

Status note:

- checked items below are verified in the local `graycode-eco` workspace
- unchecked items are intentionally reserved for external branch / release /
  publication state that this workspace audit cannot prove

### Product and dependency graph

- [x] `graycode` is the only primary end-user product in the ecosystem
- [x] Graycode coordinates engines; engines do not import each other
- [x] SDKs and community skills consume Graycode public surfaces, not engines directly
- [x] `graycode-platform` and other company/platform repos stay outside Graycode runtime dependencies

### Forbidden edges stay forbidden

- [x] no support repo imports `graycode/internal/*`
- [x] no support repo imports removed `graycode/shared/types`
- [x] no SDK/skills repo references support engines as primary dependencies
- [x] Graycode production code imports Eyrie only through `eyrie/engine`
- [x] Graycode's graph/projection imports are documented as explicit integration
      surfaces and do not create engine-to-engine dependencies

### `eagle` (implemented packages only)

These packages are in scope for v1:

- [x] `types/` — severity and finding vocabulary
- [x] `review/` — neutral review results
- [x] `verify/` — neutral verification reports
- [x] `tools/` — persisted tool call/result contracts
- [x] `events/` — normalized audit/swift event subset used by Graycode
- [x] `policy/` — permission and guardian decision contracts

Adoption bar:

- [x] `kestrel` and `merlin` import contracts for shared severity/findings and expose boundary adapters
- [x] `shrike/types` compatibility shim removed from the local ecosystem
- [x] `eyrie`, `harrier`, and `swift` remain contract-free unless they gain a true cross-repo type

### Graycode integration seams

- [x] session persistence uses `eagle/tools`, not lower-level provider tool types
- [x] review persistence and merlin/review bridge paths use neutral `review` / `verify` contracts
- [x] Graycode owns runtime DTOs in `internal/types` and translates them to `eyrie/engine` in `internal/engine`
- [x] `graycode swift ...` remains a Graycode-mounted subcommand, not a competing product surface

### Enforcement

- [x] Graycode CI runs ecosystem, shared-types, eyrie-client, and peer-coupling guards
- [x] each support repo runs `check-ecosystem-boundaries.sh` in CI
- [x] Go SDK runs consumer boundary guard in CI
- [x] Python SDK and community skills run consumer boundary guards in CI
- [x] architecture docs use the canonical 15-repository manifest and distinguish
      repository directories from product labels
- [x] lefthook strips `Co-authored-by:` trailers so commits list only the human author

### Ship state

- [ ] local architecture commits are pushed and merged to upstream default branches
- [ ] open architecture PRs for engines, consumers, and Graycode integration are merged to `main`
- [ ] published module versions used by Graycode match the merged contract changes

## Explicit non-goals for v1

Do not block v1 on any of the following:

- `eagle/sessions` or `eagle/engines`
- moving every Graycode internal event struct into `eagle/events`
- moving swift timeline/event models out of `swift`
- unifying runtime `internal/types` DTOs and persisted contracts into one type
- forcing every engine into the same integration depth (library vs subcommand vs service)
- deep static import analysis for Python SDK or markdown-only skills repos

## Governance after v1

Add a new `eagle` package only when **all** of these are true:

1. at least two repos need the same stable type or envelope
2. the type is vocabulary or DTO, not runtime logic
3. the owning repo cannot keep the type local without recreating cross-repo coupling
4. the addition is documented in `eagle-spec.md` before code lands

Remove compatibility shims only when:

1. grep/guards show zero remaining importers
2. the removal is called out in the repo CHANGELOG or migration note

## Verify locally

From `graycode`:

```bash
make ecosystem-guard contracts-guard eyrie-client-guard eyrie-engine-guard peer-guard
go test ./internal/testaudit/... -count=1
go test ./... -count=1
```

From each support repo:

```bash
bash ./scripts/check-ecosystem-boundaries.sh
go test ./... -count=1
```

## Related docs

- `graycode-product-architecture.md` — target shape and phases
- `graycode-dependency-rules.md` — allowed and forbidden edges
- `eagle-spec.md` — contract inventory and planned packages
- `../plans/eagles-migration-backlog.md` — migration history and follow-ups
