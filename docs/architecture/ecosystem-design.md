# GrayCode Ecosystem — Independent Repos, Connected Design

## Purpose

This document is the source of truth for how the GrayCodeAI ecosystem repos stay
**independent** (own git repo, version, and release cadence) while remaining
**connected** into one working product. It replaces the previous submodule model.

## Repo map

```
graycode-eco/  (workspace root — a plain folder, NOT a git repo)
│
├── hawk                 # primary product (CLI + orchestration + policy)
├── eyrie                # support engine — provider runtime
├── yaad                 # support engine — memory
├── tok                  # support engine — token budgeting
├── trace                # support engine — tracing/provenance
├── sight                # support engine — review findings
├── inspect              # support engine — verification findings
├── hawk-core-contracts  # shared leaf — neutral cross-repo DTOs
├── hawk-mcpkit          # shared leaf — MCP server scaffolding
│
├── hawk-sdk-go          # extension — public Go SDK (independent)
├── hawk-sdk-python      # extension — public Python SDK (independent)
├── hawk-community-skills# extension — skills/recipes (independent)
├── hawk-cloud           # Cloudflare Worker — hosted control plane (independent)
├── hawk-graph           # Node dashboard — dev tooling (independent)
└── graycode-core        # company web/backend monorepo (independent, not Hawk runtime)
```

## How "independent but connected" works

### 1. Connectivity via a single shared leaf: `hawk-core-contracts`

Every repo that needs to speak to another uses `hawk-core-contracts` as the one
shared vocabulary. No engine imports another engine. No engine imports `hawk`.
`hawk` is the single fan-in that orchestrates all six engines.

```
                    hawk   (only orchestrator)
                 /   |   \   \   \   \
            eyrie yaad tok trace sight inspect     ← engines: no peer imports
                |    |    |    |    |     |
                +----+----+----+----+-----+        ← connect only through the leaf
                          hawk-core-contracts       (and hawk-mcpkit for MCP helpers)
```

### 2. Local dev: workspace `go.work` (no submodules)

`hawk/go.work` lists the siblings as `../<repo>`:

```go
go 1.26.6
use (
    .
    ../eyrie
    ../yaad
    ../tok
    ../trace
    ../sight
    ../inspect
    ../hawk-core-contracts
    ../hawk-mcpkit
)
```

- No `git submodule`s, no committed `replace` directives.
- Clone all repos into `graycode-eco/`, run `make setup` in `hawk` to regenerate
  `go.work`, then `make sync` / `go work sync`.
- Local edits in any sibling are picked up immediately by hawk's build.

### 3. Published connectivity: Go module versions

Standalone and module-mode builds (Docker, released consumers) resolve the engine
versions pinned in `hawk/go.mod` from the module proxy. Each engine is released
independently with its own semver tag. `hawk-core-contracts` is released first,
then engines bump to it, then hawk bumps to the engines.

Release order: **contracts → engines → hawk.**

### 4. HTTP connectivity (non-Go components)

Components outside the Go module graph connect over public APIs:

- **`hawk-cloud`** (Cloudflare Worker) — hosted control plane (tenancy, usage,
  audit, optional sync). Hawk syncs to it via fail-open, user-approved HTTP
  (`POST /v1/graph/sync`, usage events). Needed only when a user opts into cloud.
- **`graycode-core`** — the GrayCode web/backend monorepo. Not a Hawk runtime
  dependency. It consumes `hawk-cloud`'s public API; hawk sends only opt-in,
  fail-open telemetry to it over HTTP (ADR-0001). Never a Go import.
- **`hawk-graph`** (Node dashboard) — dev tooling that reads architecture data;
  no Go module link.

## Enforcement guards (scripts + CI)

| Guard | Script / target |
|---|---|
| No engine→engine imports | `check-support-repo-coupling.sh` |
| No engine→hawk imports | `check-ecosystem-boundaries.sh` |
| No legacy `shared/types` | `check-shared-types-imports.sh` |
| Eyrie only via `engine` facade | `check-eyrie-client-imports.sh`, `check-eyrie-engine-boundary.sh` |
| No committed `replace` | `check-no-replace-directives.sh` |
| go.mod version reachable from sibling HEAD | `check-submodule-release-parity.sh` (`make release-parity`) |

## Best practices

- Bump `hawk-core-contracts` first; keep all engines on the same latest tag at
  each ecosystem release to avoid version skew.
- Never commit `replace` directives into `go.mod`.
- Add a CI integration matrix so each engine builds against the latest contracts
  and one job compiles hawk + all engines together.
