# Hawk Current vs Proposed Architecture

## Purpose

This document explains the connected repository design. The canonical
machine-readable inventory is [`hawk/ecosystem.yaml`](../../ecosystem.yaml);
this document must not maintain a second repository list.

The repository directory and product-name distinction is intentional:

| Directory/repository | Product or role |
|---|---|
| `hawk` | Hawk product and orchestration root |
| `eyrie` | Eyrie provider runtime |
| `harrier` | Harrier memory engine |
| `shrike` | Shrike token/context engine |
| `swift` | Swift provenance engine |
| `kestrel` | Kestrel review engine |
| `merlin` | Merlin verification engine |
| `eagle` | Shared neutral contracts |
| `falcon` | Shared MCP server kit |
| `sparrow` | Go SDK |
| `robin` | Python SDK |
| `wren` | TypeScript SDK |
| `starling` | Community skills and extensions |
| `owl` | Architecture visualization tooling |
| `graycode-platform` | GrayCode web, BFF, and Hawk Cloud deployment |

## Current workspace

All 15 directories are independent Git repositories in the plain
`graycode-eco` workspace. The parent [`go.work`](../../../go.work) connects
only the nine Go modules marked `workspace: true` in the manifest:

```text
graycode-eco/
├── hawk                 # product / composition root
├── eagle                # neutral contracts foundation
├── falcon               # MCP foundation
├── eyrie                # Eyrie provider engine
├── harrier              # Harrier memory engine
├── shrike               # Shrike context engine
├── swift                # Swift engine
├── kestrel              # Kestrel review engine
├── merlin               # Merlin verification engine
├── sparrow              # Go SDK
├── robin                # Python SDK
├── wren                 # TypeScript SDK
├── starling             # skills/extensions
├── owl                  # architecture tooling
└── graycode-platform    # web/BFF/Hawk Cloud platform
```

There is no `hawk/external` vendor tree in the current workspace. Local
development uses sibling checkouts and `go.work`; standalone builds use the
published versions pinned in each `go.mod`.

## Current connected design

```text
sparrow / robin / wren ── HTTP/OpenAPI ──> hawk <── skill surface ── starling
                                             │
                                             ├── eyrie/engine
                                             ├── harrier       (Harrier)
                                             ├── shrike        (Shrike)
                                             ├── swift/cli     (Swift)
                                             ├── kestrel       (Kestrel)
                                             ├── merlin        (Merlin)
                                             └── eagle         (contracts)

engines ──> eagle
harrier / kestrel / merlin ──> falcon ──> mark3labs/mcp-go

hawk ── optional, fail-open HTTP ──> graycode-platform/apps/worker
web ──> graycode-platform/apps/bff ── private Service Binding ──> worker
worker ──> D1 + Queue + R2

owl <── canonical manifest and read-only generated architecture artifacts
```

The compile-time graph is deliberately one-way:

- Hawk is the only orchestrator and product integration root.
- Engines are peers and do not import Hawk internals or one another.
- Eagle contains only neutral, implementation-light contracts.
- Falcon contains only reusable MCP transport/handler scaffolding.
- SDKs consume Hawk's daemon API contract; they do not import engines.
- Starling provides content and extension metadata through Hawk's skill surface.
- GrayCode Platform is outside the Hawk runtime module graph.

## Current versus proposed

At the repository level, the proposed architecture is already implemented:
independent repositories, a canonical manifest, sibling Go workspace wiring,
contract parity checks, and Hawk-centered dependency direction all exist.

The remaining proposed work is boundary refinement rather than repository
reorganization:

1. publish the Eagle-migrated Eyrie revision and update Hawk's standalone pin;
2. remove the transitional `hawk-core-contracts` dependency from the published
   module graph;
3. decide whether graph/projection packages should remain explicit Hawk
   integration exceptions or be hidden behind engine-owned facades;
4. keep generated Owl inventory and architecture documents synchronized with
   `ecosystem.yaml`.

## Allowed dependency edges

```text
hawk -> eyrie / harrier / shrike / swift / kestrel / merlin / eagle
engines -> eagle                  # only for shared contracts
harrier / kestrel / merlin -> falcon
sparrow / robin / wren -> Hawk public HTTP/OpenAPI surface
starling -> Hawk skill/plugin surface
graycode-platform <-> Hawk via authenticated HTTP/Service Binding only
```

The product names `Harrier`, `Shrike`, `Swift`, `Kestrel`, and `Merlin` are user-facing
labels. Dependency checks and workspace tooling must use their repository
directories and Go module paths.

## Forbidden edges

```text
engine -> engine
engine -> hawk/internal/*
SDK -> engine
skills -> engine
any Hawk engine -> graycode-platform code
any Go module -> graycode-platform code
```

The sanctioned platform connection is runtime-only and HTTP-based: Hawk may
send explicitly enabled usage or graph requests to the deployed Hawk Cloud
Worker, and local execution must remain usable when that request fails.

## Recommendation

Keep the 15-repository structure represented by `ecosystem.yaml`. Treat
`graycode-platform` as the single non-runtime platform repository, with its
`apps/worker` deployment named `graycode-cloud` and its `apps/bff` deployment
serving browser identity and the private cloud gateway. Do not create or
reintroduce repositories for product labels (`Harrier`, `Shrike`, `Swift`, `Kestrel`,
or `Merlin`) unless the manifest is changed deliberately with a corresponding
migration.
