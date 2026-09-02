# Hawk Product Architecture

## Product statement

Hawk is the model-agnostic main AI coding-agent CLI from GrayCodeAI.

`graycode-eco` is only the local parent folder used to develop the independent
repositories. It is not a monorepo or a runtime product.

Hawk is the only primary product surface in the graycode-eco ecosystem. The support repos exist to power Hawk, not to compete with it as standalone products.

For model execution specifically: **Hawk is the face and composition layer;
Eyrie is the engine.** Hawk owns the conversation and product experience while
the `eyrie/engine` facade owns the complete provider path from credential and
catalog state through model selection and normalized generation/streaming.

## Goals

- Keep Hawk model agnostic.
- Keep Hawk CLI-first and local-first.
- Keep provider integration pluggable.
- Keep support engines isolated from each other.
- Make swift, review, and verification first-class.
- Keep the design ready for a future hosted layer without making cloud a dependency.

## Target repo set

- `hawk`
- `eyrie`
- `harrier`
- `shrike`
- `swift`
- `kestrel`
- `merlin`
- `sparrow`
- `robin`
- `starling`
- `eagle`
- `falcon`
- `wren`
- `owl`
- `graycode-platform` (outside the Hawk runtime module graph)

Directory names are authoritative for dependencies: `harrier` is Harrier,
`shrike` is Shrike, `swift` is Swift, `kestrel` is Kestrel, and `merlin` is
Merlin. The product labels remain useful in CLI and user-facing documentation.

## Runtime architecture

```text
Users / SDKs / Skills
        |
        v
      HAWK
        |
        +-----------------------------+
        |                             |
        v                             v
 core execution                 trust / quality
 eyrie  harrier  shrike              swift  kestrel  merlin
        \    |    /                 \    |    /
         \   |   /                   \   |   /
          +--+--+---------------------+--+--+
                     |
                     v
            eagle
```

## Current vs proposed

Current implementation in the workspace:

```text
hawk
  -> eyrie
  -> harrier
  -> shrike
  -> swift
  -> kestrel
  -> merlin
  -> eagle

support engines
  -> eagle when they need shared contracts
  -> falcon where MCP serving is shared
  x-> Hawk internals or each other
```

Proposed steady-state architecture:

```text
SDKs / Skills / future integrations
                |
                v
              Hawk
                |
   +------------+------------+------------+------------+------------+------------+
   |            |            |            |            |            |            |
   v            v            v            v            v            v            v
 Eyrie        Harrier         Shrike         Swift        Kestrel       Merlin    public APIs
   \            |            |            |            |            /
    +-----------+------------+------------+------------+-----------+
                                 |
                                 v
                      eagle
```

This means:

- Hawk is the product and orchestration boundary
- all six engines stay at the same architectural level
- engines remain independent from each other
- shared cross-repo vocabulary lives below them in `eagle`
- SDKs and community skills consume Hawk, not the engines directly
- `graycode-platform` provides the optional hosted plane through HTTP and a
  private Service Binding; it is not imported by Hawk or any engine

## Hawk responsibilities

Hawk owns:

- CLI entrypoints
- session lifecycle
- orchestration
- workflow control
- tool routing
- permission and policy enforcement
- user model preference and task-semantic model intent
- engine coordination
- public integration surfaces

Hawk does not own:

- provider-specific implementation details
- engine-specific business logic
- future company-wide auth/billing/platform concerns

## Engine responsibilities

### `eyrie`
- stable host control and generation facade (`eyrie/engine`)
- credential storage and safe credential status
- catalog discovery and model metadata
- concrete provider/deployment selection
- provider adapters
- request/response normalization
- streaming
- retries/timeouts/fallbacks
- low-level provider registry and compatibility logic behind the engine facade

### `harrier`
- session and long-term memory
- retrieval hooks
- summaries and persistence contracts

### `shrike`
- token budgeting
- context ranking
- packing and truncation
- model-ready context assembly inputs

### `swift`
- event capture
- replay records
- provenance
- audit visibility

### `kestrel`
- review findings
- risk detection
- code quality analysis
- review-engine-local output converted into shared `eagle/review` contracts at product boundaries

### `merlin`
- verification checks
- test/assertion normalization
- final pass/fail validation
- verification-engine-local output converted into shared `eagle/verify` contracts at product boundaries

## Primary runtime flow

1. User invokes `hawk`.
2. Hawk loads product settings, policy, and workspace state, then creates an
   Eyrie Engine with effective per-instance custom gateway settings.
3. Hawk creates or resumes a session.
4. Hawk asks `shrike` for context assembly.
5. Hawk asks `harrier` for relevant memory.
6. Hawk routes provider execution through `eyrie`.
7. Hawk invokes tools and records actions through `swift`.
8. Hawk invokes `kestrel` when review should run.
9. Hawk invokes `merlin` when verification should run.
10. Hawk persists results and returns output to the user.

At step 6, Hawk passes intent and Hawk-owned conversation DTOs through its
adapter. Eyrie loads provider/catalog/credential state, resolves the gateway,
and returns normalized events. No production Hawk package imports a lower
Eyrie package, and no Eyrie engine DTO is used as Hawk's persistent or CLI
schema.

## Implementation phases

### Phase 1
- freeze architecture rules
- document repo roles
- define shared contracts inventory

### Phase 2
- add `eagle`
- move shared types out of Hawk internals

Status:
- completed
- shared contracts now exist for `types`, `review`, `verify`, `tools`, `events`, and `policy`

### Phase 3
- remove engine imports of Hawk internals
- remove engine-to-engine coupling

Status:
- completed for current workspace boundaries
- local/CI guards now block support-repo imports of `hawk/internal/*` and removed legacy `hawk/shared/types`

### Phase 4
- harden orchestration boundaries in Hawk
- formalize provider, swift, review, and verify integration points

Status:
- completed for the local runtime boundary
- Hawk owns runtime DTOs and review/verify product-boundary contracts
- Hawk's `ChatClient` anti-corruption port translates only to `eyrie/engine`
- all lower-level Eyrie production imports are forbidden by shell guards and meta-audit tests

### Phase 5
- align SDKs and skills to Hawk public interfaces only

Status:
- policy is now explicit and guarded in Hawk docs
- `sparrow` is covered by the support-repo coupling guard so it cannot grow
  direct engine imports
- broader non-Go consumer enforcement remains future work

### Phase 6
- remove legacy `hawk/shared/types`
- keep import guards in place so the old path cannot return

Status:
- completed in the local ecosystem

## Done criteria

The architecture is in good shape when:

- `hawk` is the only product surface
- engines depend only on `eagle`
- shared types no longer live in Hawk internals as a cross-repo API
- provider abstraction is stable
- swift, review, and verification are part of the standard runtime flow
- deprecated compatibility surfaces have a documented removal path and active guardrails
