# Hawk Dependency Rules

## Required graph

These edges always hold:

```text
hawk -> eyrie
hawk -> yaad
hawk -> tok
hawk -> trace
hawk -> sight
hawk -> inspect
hawk -> hawk-core-contracts

hawk-sdk-go -> hawk public API/contracts
hawk-sdk-python -> hawk public API/contracts
hawk-community-skills -> hawk plugin/skill API
```

Product shape:

```text
top
  hawk-sdk-go / hawk-sdk-python / hawk-community-skills
                         |
                         v
                       hawk
                         |
      +------------------+------------------+
      |         |         |        |        |
      v         v         v        v        v
    eyrie     yaad      tok      trace    sight    inspect
      \         |         |        |        |         /
       +--------+---------+--------+--------+--------+
                               |
                               v
                    hawk-core-contracts
bottom
```

## Contract edges

An engine depends on `hawk-core-contracts` **only when it produces or consumes a
cross-repo contract** (a shared finding, severity, event, etc.). Engines that
expose no cross-repo type stay contract-free — adding the dependency "to be
consistent" is a violation of "keep the graph minimal", not an improvement.

Current state (keep in sync with the code; the boundary guards enforce the
*forbidden* edges below, not these contract edges):

```text
sight   -> hawk-core-contracts   # severity/finding vocabulary
inspect -> hawk-core-contracts   # severity/finding vocabulary
eyrie   -> (none)   # provider/transport types are eyrie-local
yaad    -> (none)   # memory event types are yaad-local
trace   -> (none)   # trace/redaction event types are trace-local
```

If `eyrie`, `yaad`, or `trace` later needs to emit or accept a shared
finding/event, the type moves into `hawk-core-contracts` first and the engine
adds the contract edge then — not before.

## Forbidden graph

```text
engine -> engine
engine -> hawk/internal/*
engine -> hawk/shared/* as a public dependency
sdk -> engine
skills -> engine
```

## Rules

### 1. Hawk is the orchestrator
Only Hawk coordinates the support engines.

### 2. Engines are peers
Engines may share concepts through contracts, but not by importing each other.

### 3. Shared types belong in contracts
Anything used across repos must move to `hawk-core-contracts`.

### 4. Public integrations go through Hawk
SDKs and skills must use Hawk public APIs, contracts, or plugin surfaces.

### 5. Provider logic stays behind the Eyrie engine boundary
Provider-specific code should not leak into memory, review, verify, or trace engines.
Hawk production code imports Eyrie only through `github.com/GrayCodeAI/eyrie/engine`.

### 6. Hawk schemas stay Hawk-owned
Hawk's conversation persistence and CLI/JSON output are explicit projections,
not aliases or direct serialization of Eyrie engine DTOs.

### 7. Engine configuration is instance-scoped
Hawk supplies effective custom gateways while constructing an Engine. Do not
mutate Eyrie process-global gateway state from product code.

## Current cleanup targets

Based on current local structure:

- `sight -> hawk/shared/types` removed
- `inspect -> hawk/shared/types` removed
- `tok/types` compatibility shim removed
- keep support engines peer-isolated as new features are added

## Enforcement

These were previously "ideas"; they are now implemented:

- each support repo documents its import boundary in its README
  ("Ecosystem Boundaries" section)
- CI runs `scripts/check-ecosystem-boundaries.sh` in every support repo, and
  Hawk additionally runs `check-shared-types-imports.sh`,
  `check-eyrie-client-imports.sh`, `check-eyrie-engine-boundary.sh`, and
  `check-support-repo-coupling.sh`
- `hawk-core-contracts` is kept minimal (leaf module, no external dependencies)

The Hawk boundary guards use ripgrep when available and fall back to recursive
grep, so a minimal CI image cannot turn a missing scanner into a vacuous pass.
