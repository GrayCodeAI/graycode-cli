# Hawk Product Architecture

## Product statement

Hawk is a model-agnostic AI coding agent CLI from GrayCodeAI.

Hawk is the only primary product surface in the Hawk ecosystem. The support repos exist to power Hawk, not to compete with it as standalone products.

## Goals

- Keep Hawk model agnostic.
- Keep Hawk CLI-first and local-first.
- Keep provider integration pluggable.
- Keep support engines isolated from each other.
- Make trace, review, and verification first-class.
- Keep the design ready for a future hosted layer without making cloud a dependency.

## Target repo set

- `hawk`
- `eyrie`
- `yaad`
- `tok`
- `trace`
- `sight`
- `inspect`
- `hawk-sdk-go`
- `hawk-sdk-python`
- `hawk-community-skills`
- `hawk-core-contracts`

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
 eyrie  yaad  tok              trace  sight  inspect
        \    |    /                 \    |    /
         \   |   /                   \   |   /
          +--+--+---------------------+--+--+
                     |
                     v
            hawk-core-contracts
```

## Current vs proposed

Current implementation in this repo:

```text
hawk
  -> eyrie
  -> yaad
  -> tok
  -> trace
  -> sight
  -> inspect
  -> hawk-core-contracts

support engines
  -> hawk-core-contracts only when they need shared contracts
  x-> each other
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
 Eyrie        Yaad         Tok         Trace        Sight       Inspect    public APIs
   \            |            |            |            |            /
    +-----------+------------+------------+------------+-----------+
                                 |
                                 v
                      hawk-core-contracts
```

This means:

- Hawk is the product and orchestration boundary
- all six engines stay at the same architectural level
- engines remain independent from each other
- shared cross-repo vocabulary lives below them in `hawk-core-contracts`
- SDKs and community skills consume Hawk, not the engines directly

## Hawk responsibilities

Hawk owns:

- CLI entrypoints
- session lifecycle
- orchestration
- workflow control
- tool routing
- permission and policy enforcement
- provider selection
- engine coordination
- public integration surfaces

Hawk does not own:

- provider-specific implementation details
- engine-specific business logic
- future company-wide auth/billing/platform concerns

## Engine responsibilities

### `eyrie`
- provider adapters
- request/response normalization
- streaming
- retries/timeouts/fallbacks
- low-level provider registry and compatibility logic behind Hawk-owned transport adapters

### `yaad`
- session and long-term memory
- retrieval hooks
- summaries and persistence contracts

### `tok`
- token budgeting
- context ranking
- packing and truncation
- model-ready context assembly inputs

### `trace`
- event capture
- replay records
- provenance
- audit visibility

### `sight`
- review findings
- risk detection
- code quality analysis
- review-engine-local output converted into shared `hawk-core-contracts/review` contracts at product boundaries

### `inspect`
- verification checks
- test/assertion normalization
- final pass/fail validation
- verification-engine-local output converted into shared `hawk-core-contracts/verify` contracts at product boundaries

## Primary runtime flow

1. User invokes `hawk`.
2. Hawk loads config, policy, provider settings, and workspace state.
3. Hawk creates or resumes a session.
4. Hawk asks `tok` for context assembly.
5. Hawk asks `yaad` for relevant memory.
6. Hawk routes provider execution through `eyrie`.
7. Hawk invokes tools and records actions through `trace`.
8. Hawk invokes `sight` when review should run.
9. Hawk invokes `inspect` when verification should run.
10. Hawk persists results and returns output to the user.

## Implementation phases

### Phase 1
- freeze architecture rules
- document repo roles
- define shared contracts inventory

### Phase 2
- add `hawk-core-contracts`
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
- formalize provider, trace, review, and verify integration points

Status:
- substantially completed
- Hawk now owns runtime DTOs, transport config/provider seams, and review/verify product-boundary contracts
- direct `eyrie/client` usage is restricted to adapter edges and enforced by shell guards plus meta-audit tests

### Phase 5
- align SDKs and skills to Hawk public interfaces only

Status:
- policy is now explicit and guarded in Hawk docs
- `hawk-sdk-go` is covered by the support-repo coupling guard so it cannot grow
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
- engines depend only on `hawk-core-contracts`
- shared types no longer live in Hawk internals as a cross-repo API
- provider abstraction is stable
- trace, review, and verification are part of the standard runtime flow
- deprecated compatibility surfaces have a documented removal path and active guardrails
