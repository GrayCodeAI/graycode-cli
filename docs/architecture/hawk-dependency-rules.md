# Hawk Dependency Rules

`graycode-eco` is only a local parent folder. `hawk` is the main CLI and the
single orchestration root. Repository names below use directory/module names;
product labels are shown in parentheses where they differ.

## Required graph

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

harrier / kestrel / merlin ──> falcon ──> mark3labs/mcp-go
engines ──> eagle when a shared contract is required
```

The canonical list of all 15 repositories is in
[`hawk/ecosystem.yaml`](../../ecosystem.yaml). `owl/ecosystem.json` is a
generated projection of that list.

## Contract edges

An engine may depend on Eagle only when it produces or consumes a cross-repo
contract such as a finding, severity, event, or graph fact. Current Go module
consumers are tracked by `ecosystem.yaml` and checked for Eagle version parity.

Falcon is a separate foundation for shared MCP transport and handler patterns.
It remains upstream-only and must not import Hawk, Eagle, or an engine.

## Forbidden graph

```text
engine -> engine
engine -> hawk/internal/*
engine -> hawk/shared/*
SDK -> engine
skills -> engine
any Go module -> graycode-platform code
```

## Rules

### 1. Hawk is the orchestrator

Only Hawk coordinates the support engines and owns the user-facing CLI,
daemon, sessions, tools, policy, and product workflows.

### 2. Engines are peers

Engines may share concepts through Eagle contracts, but may not import each
other. Product names do not change this rule: `harrier` is Harrier, `shrike` is
Shrike, `swift` is Swift, `kestrel` is Kestrel, and `merlin` is Merlin.

### 3. Provider logic stays behind Eyrie

Hawk production code reaches provider credentials, catalogs, routing, and
transport only through `github.com/GrayCodeAI/eyrie/engine`. This is enforced
by shell and AST/package-graph guards.

### 4. Hawk schemas stay Hawk-owned

Hawk conversation persistence and CLI/JSON output are explicit projections,
not aliases or direct serialization of Eyrie DTOs.

### 5. Graph integrations are explicit surfaces

Hawk currently imports selected engine-owned graph/projection packages for
memory, token, review, and verification capture. These are integration
exceptions, not peer-engine dependencies. If replaceability requires stronger
isolation, move those calls behind engine-owned facades; do not expose more
storage or implementation types.

### 6. Cloud is runtime-only

`graycode-platform` is outside the Hawk Go module graph. Hawk may call the
deployed `graycode-cloud` Worker over authenticated HTTP for optional usage or
explicit graph synchronization. Platform failures must not block local CLI
execution.

### 7. Engine configuration is instance-scoped

Hawk supplies effective gateway settings while constructing an Eyrie Engine.
Product code must not mutate Eyrie process-global gateway state.

## Enforcement

Hawk CI runs manifest validation, no-local-replace checks, Eagle parity,
support-repository coupling, Eyrie facade checks, internal-layer checks, and
the AST package-boundary audit. Support repositories run their own boundary
guards. SDK contract tests compare their OpenAPI snapshots with Hawk's public
daemon contract.
