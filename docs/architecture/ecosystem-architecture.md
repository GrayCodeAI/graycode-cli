# GrayCode Ecosystem Architecture

This document describes how the independent repositories around the Hawk main
CLI connect. `graycode-eco` is only the local parent folder; it is not a Git
repository, product, or runtime module.

## Repository boundary

| Repository | Responsibility | Connection to Hawk |
|---|---|---|
| `hawk` | Main CLI, daemon, orchestration, policy, and public API | Main product and integration root |
| `eagle` | Neutral cross-repository contracts | Go dependency of Hawk and selected engines |
| `falcon` | Shared MCP transport and handler scaffolding | Go dependency of MCP-serving engines |
| `eyrie` | Provider credentials, catalog, routing, transport, streaming | Hawk consumes `eyrie/engine` |
| `harrier` | Harrier memory and retrieval engine | Hawk integration |
| `shrike` | Shrike token budgeting and context compression | Hawk integration |
| `swift` | Swift provenance and replay engine | Hawk consumes `swift/cli` |
| `kestrel` | Kestrel source-review engine | Hawk integration |
| `merlin` | Merlin verification/target-inspection engine | Hawk integration |
| `sparrow` | Go SDK | Hawk daemon API/OpenAPI consumer |
| `robin` | Python SDK | Hawk daemon API/OpenAPI consumer |
| `wren` | TypeScript SDK | Hawk daemon API/OpenAPI consumer |
| `starling` | Community skills and extensions | Hawk skill/plugin consumer |
| `owl` | Architecture visualization tooling | Reads the generated manifest/artifacts |
| `graycode-platform` | Web application, browser BFF, and Hawk Cloud Worker | Optional HTTP/Service Binding plane |

Product names are labels, not directory names: `harrier` is Harrier, `shrike` is
Shrike, `swift` is Swift, `kestrel` is Kestrel, and `merlin` is Merlin.

## Dependency and runtime topology

```text
sparrow / robin / wren ── HTTP/OpenAPI ──> hawk <── skill surface ── starling
                                             │
                                             ├── eyrie/engine
                                             ├── harrier       (Harrier)
                                             ├── shrike        (Shrike)
                                             ├── swift/cli     (Swift)
                                             ├── kestrel       (Kestrel)
                                             ├── merlin        (Merlin)
                                             └── eagle

engines ──> eagle when a shared contract is required
harrier / kestrel / merlin ──> falcon ──> mark3labs/mcp-go

hawk ── optional authenticated HTTP ──> graycode-platform/apps/worker
web ──> graycode-platform/apps/bff ── private Service Binding ──> worker
worker ──> D1 + Queue + R2

owl <── hawk/ecosystem.yaml via generated ecosystem.json and analysis artifacts
```

The Worker deployment is named `graycode-cloud`, but it is not a separate
repository. It lives in `graycode-platform/apps/worker`; browser identity and
the gateway live in `graycode-platform/apps/bff`.

## Workspace policy

The parent `go.work` connects the nine Go repositories marked `workspace: true`
in `hawk/ecosystem.yaml`: Hawk, Eagle, Falcon, and the six engines. The SDKs,
Starling, Owl, and GrayCode Platform are independent non-Go/API/tooling repos.

Local sibling resolution is a development convenience. Each repository keeps
its own Git history, version, CI, and release cadence. Standalone Hawk builds
use published module versions with `GOWORK=off`.

## Architecture invariants

1. Hawk is the only main product CLI and orchestration root.
2. Engines do not import Hawk internals or peer engines.
3. Shared neutral Go vocabulary belongs in Eagle.
4. SDKs and skills consume Hawk public surfaces, never engine internals.
5. No Go module imports `graycode-platform`.
6. Hawk may use the platform only through authenticated, optional runtime HTTP.
7. Local execution remains usable when the platform is unavailable.
8. The canonical repository list is `hawk/ecosystem.yaml`; Owl is a projection.

## Current versus proposed

The proposed repository shape is already present locally. Remaining work is
release and boundary refinement: publish the Eagle-compatible Eyrie revision,
remove its transitional legacy contract dependency from the standalone graph,
and decide whether Hawk's graph/projection integrations should be hidden behind
engine-owned facades.

The executable checklist is maintained in
`docs/architecture/ecosystem-implementation-plan.md`.
