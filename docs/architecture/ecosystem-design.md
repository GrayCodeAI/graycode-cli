# GrayCode Ecosystem — Independent Repositories, Connected Design

## Authority

[`graycode/ecosystem.yaml`](../ecosystem.yaml) is the canonical repository
inventory. It defines the 15 repositories, their directory names, product
labels, Go modules, workspace membership, and Eagle-contract participation.
[`owl/ecosystem.json`](../../owl/ecosystem.json) is a generated snapshot and
must match it exactly.

Every directory in `graycode-eco` is an independent Git repository. The plain
workspace root provides local coordination; it is not itself a Git repository.

## Repository map

```text
graycode-eco/
├── graycode                 # product, CLI, daemon, orchestration, policy
├── eagle                # shared neutral contracts
├── falcon               # shared MCP transport/handler kit
├── graycode-router                # provider runtime (product label: GraycodeRouter)
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
└── graycode-platform    # web, browser BFF, and Graycode Cloud Worker
```

## Compile-time dependency graph

```text
sparrow / robin / wren ── Graycode daemon API ──> graycode <── skill API ── starling
                                             │
                                             ├── graycode-router/engine
                                             ├── harrier       (Harrier)
                                             ├── shrike        (Shrike)
                                             ├── swift/cli     (Swift)
                                             ├── kestrel       (Kestrel)
                                             ├── merlin        (Merlin)
                                             └── eagle

graycode-router / harrier / shrike / swift / kestrel / merlin ──> eagle (as needed)
harrier / kestrel / merlin ──> falcon ──> mark3labs/mcp-go
```

Graycode is the only orchestration root. Engines are peers: they do not import
Graycode internals and do not import one another. Eagle owns neutral contracts;
Falcon owns reusable MCP scaffolding. SDKs are API consumers, not Go imports
of the engines.

## Local Go workspace

The parent [`go.work`](../../../go.work) connects the nine repositories marked
`workspace: true`: Graycode, Eagle, Falcon, and the six engine repositories. The
SDKs, Starling, Owl, and GrayCode Platform remain independent consumers or
tooling and are not part of the Go workspace.

Local sibling replacements exist only in `go.work`. Repository `go.mod` files
remain publishable and pin released or reachable module versions. Release and
CI must test both workspace mode and `GOWORK=off` module mode.

## Runtime/API connectivity

```text
Graycode local runtime
  ├── GraycodeRouter provider generation
  ├── Harrier memory and retrieval
  ├── Shrike token budgeting/compression
  ├── Swift swift/provenance
  ├── Kestrel review
  └── Merlin verification

Graycode ── optional authenticated HTTP ──> graycode-platform/apps/worker
web ──> graycode-platform/apps/bff ── private Service Binding ──> worker
worker ──> control-plane D1, usage Queue, and R2 archive

Owl <── manifest and read-only generated repository artifacts
```

The Worker deployment is named `graycode-cloud`, but it is an application
inside the `graycode-platform` repository, not a separate repository. The BFF
owns browser identity and forwards authenticated requests over the private
`GraycodeCloudService` binding. Graycode Cloud owns Graycode-specific projects, devices,
usage, billing, audit, and graph records.

Cloud is optional. Graycode local execution must never depend on the platform being
available; automatic usage delivery is fail-open, while graph synchronization
is an explicit user operation.

## Boundary rules

Allowed:

```text
graycode -> engine facades and Eagle contracts
engine -> Eagle when a shared contract is required
harrier / kestrel / merlin -> Falcon for MCP serving
SDK -> Graycode public HTTP/OpenAPI contract
Starling -> Graycode skill/plugin surface
Graycode -> deployed Graycode Cloud over authenticated HTTP only
```

Forbidden:

```text
engine -> engine
engine -> graycode/internal/*
SDK -> engine
skills -> engine
any Go module -> graycode-platform code
```

Graph and quality-projection packages imported by Graycode are explicit integration
surfaces. If they become replaceability-sensitive, move them behind the
corresponding engine facade rather than exporting implementation types further.

## Release order

1. Publish Eagle contract changes.
2. Publish the Eagle-compatible engine revisions.
3. Update Graycode's engine pins and verify `GOWORK=off` builds.
4. Validate SDK OpenAPI snapshots against Graycode.
5. Deploy GrayCode Platform independently after its Worker/BFF contract tests
   pass.

The current local GraycodeRouter checkout is Eagle-compatible, but Graycode's published
GraycodeRouter pin still has a transitive `graycode-core-contracts` dependency. That is a
release-order issue, not a reason to add a local `replace` directive or a
platform dependency.
