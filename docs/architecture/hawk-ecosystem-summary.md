# Hawk Ecosystem Summary

## One-line view

`hawk` is the product. Everything else in the Hawk ecosystem either powers Hawk,
extends Hawk, or provides shared contracts for Hawk.

## Final repo map

```text
top: consumers and extensions

  hawk-sdk-go       hawk-sdk-python       hawk-community-skills
         \                 |                        /
          \                |                       /
           +---------------+----------------------+
                           |
                           v
                         hawk

middle: support engines

      +---------+---------+---------+---------+---------+---------+
      |         |         |         |         |         |         |
      v         v         v         v         v         v         v
    eyrie     yaad      tok       trace     sight    inspect   public APIs

bottom: shared foundations

              hawk-core-contracts      hawk-mcpkit
```

## Dependency direction

### Product layer

- `hawk -> eyrie`
- `hawk -> yaad`
- `hawk -> tok`
- `hawk -> trace`
- `hawk -> sight`
- `hawk -> inspect`
- `hawk -> hawk-core-contracts`

### Shared-contract layer

- `engine -> hawk-core-contracts` only when a real cross-repo DTO is required
- `engine -> hawk-mcpkit` for MCP server scaffolding (currently `sight`, `inspect`)

### Extension layer

- `hawk-sdk-go -> hawk`
- `hawk-sdk-python -> hawk`
- `hawk-community-skills -> hawk`

### Forbidden edges

- `engine -> engine`
- `sdk -> engine`
- `skills -> engine`
- `engine -> hawk/internal/*`
- `hawk runtime -> graycode-core` — at compile time; opt-in, fail-open HTTP
  telemetry only, per `adr/ADR-0001-graycode-core-telemetry-edge.md`

## Repo-by-repo roles

| Repo | Layer | Role | Depends on | Must not depend on |
|---|---|---|---|---|
| `hawk` | Product | CLI, orchestration, policy, execution control, public APIs | support engines, `hawk-core-contracts` | sibling company/platform repos in runtime paths |
| `eyrie` | Support engine | provider runtime, model execution, streaming, retries | `hawk-core-contracts` only if needed | `yaad`, `tok`, `trace`, `sight`, `inspect` |
| `yaad` | Support engine | memory, retrieval, persistence of long-lived context | `hawk-core-contracts` only if needed | `eyrie`, `tok`, `trace`, `sight`, `inspect` |
| `tok` | Support engine | token budgeting, packing, truncation, context shaping | `hawk-core-contracts` only if needed | `eyrie`, `yaad`, `trace`, `sight`, `inspect` |
| `trace` | Support engine | trace capture, replay, provenance, audit trail | `hawk-core-contracts` only if needed | `eyrie`, `yaad`, `tok`, `sight`, `inspect` |
| `sight` | Support engine | review findings, code-quality/risk analysis | `hawk-core-contracts` | `eyrie`, `yaad`, `tok`, `trace`, `inspect` |
| `inspect` | Support engine | verification findings, checks, pass/fail validation | `hawk-core-contracts` | `eyrie`, `yaad`, `tok`, `trace`, `sight` |
| `hawk-core-contracts` | Foundation | shared DTOs and vocabulary | leaf module | product logic or engine implementation logic |
| `hawk-mcpkit` | Foundation | shared MCP server scaffolding (wraps `mark3labs/mcp-go`) | upstream MCP library only | engines, hawk, graycode-core |
| `hawk-sdk-go` | Consumer | Go integrations for Hawk public surfaces | Hawk public API/contracts | support engines directly |
| `hawk-sdk-python` | Consumer | Python integrations for Hawk public surfaces | Hawk public API/contracts | support engines directly |
| `hawk-community-skills` | Consumer | skills, recipes, extension packs | Hawk plugin/skill surfaces | support engines directly |

## Runtime responsibility split

### `hawk`

Owns:

- the CLI and command UX
- session lifecycle
- approval and permission policy
- tool routing
- provider selection
- engine coordination
- user-visible product APIs

### `eyrie`

Owns:

- provider adapters
- request/response normalization
- streaming transport
- retry and timeout logic

### `yaad`

Owns:

- memory retrieval
- memory persistence
- summarization support for long-lived context

### `tok`

Owns:

- token estimation
- context packing
- truncation strategy
- prompt-ready context shaping

### `trace`

Owns:

- event capture
- replay artifacts
- provenance/session evidence
- audit-grade traceability

### `sight`

Owns:

- review heuristics
- issue/finding generation
- normalized review output at the Hawk boundary

### `inspect`

Owns:

- verification heuristics
- test/assertion result normalization
- final pass/fail output at the Hawk boundary

## Why all support engines are at the same level

They are all peers because Hawk is the orchestrator.

That means:

- `eyrie` is not "above" `sight` or `inspect`
- `sight` is not "below" execution
- `inspect` is not a child of review
- `yaad` and `tok` are not shared utility packages for other engines

Hawk calls whichever engine is needed for a given turn.

## Future cloud position

Future GrayCodeAI cloud or platform work should sit above products, not inside
the support-engine mesh.

Correct future shape:

```text
GrayCodeAI company/platform
├── web/docs
├── accounts
├── billing
├── hosted control plane
├── org/admin services
└── product gateways
    ├── Hawk
    ├── Lark
    └── Gitant
```

For Hawk specifically:

- local CLI runtime should still work without GrayCode cloud
- cloud can add hosted sessions, org policy, billing, identity, telemetry, or remote execution later
- none of that should force `eyrie`, `yaad`, `tok`, `trace`, `sight`, or `inspect` to depend on platform repos

## What belongs in `graycode-core`

Good candidates:

- company website
- account system
- billing
- control plane
- docs portal
- admin and org management

Not a good candidate:

- Hawk runtime-critical engine logic
- direct support-engine dependencies

## Practical rule set

When adding new work:

1. If it is user-facing product behavior, put it in `hawk`.
2. If it is specialized execution/memory/context/review/verify/trace capability, put it in the matching support engine.
3. If multiple repos need the same DTO or vocabulary, move that contract into `hawk-core-contracts`.
4. If it is an integration surface for external developers, put it in an SDK or skill repo.
5. If it is company platform or cloud control-plane work, keep it outside Hawk runtime repos.

## Final recommendation

Keep this exact model:

- one primary product repo: `hawk`
- six peer support engines: `eyrie`, `yaad`, `tok`, `trace`, `sight`, `inspect`
- one shared contract repo: `hawk-core-contracts`
- three extension repos: `hawk-sdk-go`, `hawk-sdk-python`, `hawk-community-skills`

This is the cleanest shape for OSS clarity, production hardening, and future scale.
