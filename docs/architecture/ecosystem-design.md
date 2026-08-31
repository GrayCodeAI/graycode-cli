# GrayCode Ecosystem — Independent Repositories, Connected Design

## Authority

[`hawk/ecosystem.yaml`](../ecosystem.yaml) is the canonical repository
inventory. It defines the 15 repositories, their directory names, product
labels, Go modules, workspace membership, and Eagle-contract participation.
[`owl/ecosystem.json`](../../owl/ecosystem.json) is a generated snapshot and
must match it exactly.

Every directory in `graycode-eco` is an independent Git repository. The plain
workspace root provides local coordination; it is not itself a Git repository.

## Repository map

```text
graycode-eco/
├── hawk                 # product, CLI, daemon, orchestration, policy
├── eagle                # shared neutral contracts
├── falcon               # shared MCP transport/handler kit
├── eyrie                # provider runtime (product label: Eyrie)
├── harrier              # memory engine (product label: Harrier)
├── shrike               # token/context engine (product label: Shrike)
├── swift                # provenance engine (product label: Swift)
├── kestrel              # review engine (product label: Kestrel)
├── merlin               # verification engine (product label: Merlin)
├── sparrow              # Go SDK
├── robin                # Python SDK
├── wren                 # TypeScript SDK
├── starling             # community skills/extensions
├── owl                  # architecture visualization tooling
└── graycode-platform    # web, browser BFF, and Hawk Cloud Worker
```

## Compile-time dependency graph

```text
sparrow / robin / wren ── Hawk daemon API ──> hawk <── skill API ── starling
                                             │
                                             ├── eyrie/engine
                                             ├── harrier       (Harrier)
                                             ├── shrike        (Shrike)
                                             ├── swift/cli     (Swift)
                                             ├── kestrel       (Kestrel)
                                             ├── merlin        (Merlin)
                                             └── eagle

eyrie / harrier / shrike / swift / kestrel / merlin ──> eagle (as needed)
harrier / kestrel / merlin ──> falcon ──> mark3labs/mcp-go
```

Hawk is the only orchestration root. Engines are peers: they do not import
Hawk internals and do not import one another. Eagle owns neutral contracts;
Falcon owns reusable MCP scaffolding. SDKs are API consumers, not Go imports
of the engines.

## Local Go workspace

The parent [`go.work`](../../../go.work) connects the nine repositories marked
`workspace: true`: Hawk, Eagle, Falcon, and the six engine repositories. The
SDKs, Starling, Owl, and GrayCode Platform remain independent consumers or
tooling and are not part of the Go workspace.

Local sibling replacements exist only in `go.work`. Repository `go.mod` files
remain publishable and pin released or reachable module versions. Release and
CI must test both workspace mode and `GOWORK=off` module mode.

## Runtime/API connectivity

```text
Hawk local runtime
  ├── Eyrie provider generation
  ├── Harrier memory and retrieval
  ├── Shrike token budgeting/compression
  ├── Swift swift/provenance
  ├── Kestrel review
  └── Merlin verification

Hawk ── optional authenticated HTTP ──> graycode-platform/apps/worker
web ──> graycode-platform/apps/bff ── private Service Binding ──> worker
worker ──> control-plane D1, usage Queue, and R2 archive

Owl <── manifest and read-only generated repository artifacts
```

The Worker deployment is named `graycode-cloud`, but it is an application
inside the `graycode-platform` repository, not a separate repository. The BFF
owns browser identity and forwards authenticated requests over the private
`HawkCloudService` binding. Hawk Cloud owns Hawk-specific projects, devices,
usage, billing, audit, and graph records.

Cloud is optional. Hawk local execution must never depend on the platform being
available; automatic usage delivery is fail-open, while graph synchronization
is an explicit user operation.

## Boundary rules

Allowed:

```text
hawk -> engine facades and Eagle contracts
engine -> Eagle when a shared contract is required
harrier / kestrel / merlin -> Falcon for MCP serving
SDK -> Hawk public HTTP/OpenAPI contract
Starling -> Hawk skill/plugin surface
Hawk -> deployed Hawk Cloud over authenticated HTTP only
```

Forbidden:

```text
engine -> engine
engine -> hawk/internal/*
SDK -> engine
skills -> engine
any Go module -> graycode-platform code
```

Graph and quality-projection packages imported by Hawk are explicit integration
surfaces. If they become replaceability-sensitive, move them behind the
corresponding engine facade rather than exporting implementation types further.

## Release order

1. Publish Eagle contract changes.
2. Publish the Eagle-compatible engine revisions.
3. Update Hawk's engine pins and verify `GOWORK=off` builds.
4. Validate SDK OpenAPI snapshots against Hawk.
5. Deploy GrayCode Platform independently after its Worker/BFF contract tests
   pass.

The current local Eyrie checkout is Eagle-compatible, but Hawk's published
Eyrie pin still has a transitive `hawk-core-contracts` dependency. That is a
release-order issue, not a reason to add a local `replace` directive or a
platform dependency.
