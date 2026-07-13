# Hawk Current vs Proposed Architecture

## Purpose

This document is the single source of truth for:

- what exists in the current local workspace
- which repos are part of the Hawk product architecture
- which repos are support-only
- what the final steady-state repo graph should be

The goal is simple:

- `hawk` is the product
- support engines are independent from each other
- support engines depend on Hawk orchestration, not on sibling engines
- shared vocabulary lives in `hawk-core-contracts`
- SDKs and skills extend Hawk, not the engines directly

## Scope

This workspace contains both:

- the Hawk product ecosystem
- other GrayCodeAI company/platform repos that are not part of Hawk runtime architecture

Those should not be mixed together when making product or dependency decisions.

## Current workspace shape

The local workspace currently contains these top-level repos:

### Hawk product repos

- `hawk`
- `eyrie`
- `yaad`
- `tok`
- `trace`
- `sight`
- `inspect`
- `hawk-core-contracts`
- `hawk-mcpkit`
- `hawk-sdk-go`
- `hawk-sdk-python`
- `hawk-community-skills`

### Non-Hawk repo currently present in the same workspace

- `graycode-core`

## Current actual layout

Today, the workspace is a multi-repo development area, and `hawk` also vendors or
pins support repos under `hawk/external` for reproducible integration work.

```text
hawk-eco/
├── hawk                       # primary product repo
│   └── external/
│       ├── eyrie
│       ├── yaad
│       ├── tok
│       ├── trace
│       ├── sight
│       ├── inspect
│       └── hawk-core-contracts
├── eyrie
├── yaad
├── tok
├── trace
├── sight
├── inspect
├── hawk-core-contracts
├── hawk-mcpkit               # shared MCP server scaffolding (used by sight, inspect)
├── hawk-sdk-go
├── hawk-sdk-python
├── hawk-community-skills
└── graycode-core             # separate company/platform repo, not Hawk runtime
```

## Current runtime relationship

The intended runtime shape is already mostly reflected in code and guards:

```text
users / scripts / sdk / skills
             |
             v
           hawk
             |
   +---------+---------+---------+---------+---------+---------+
   |         |         |         |         |         |         |
   v         v         v         v         v         v         v
 eyrie     yaad      tok       trace     sight    inspect   product APIs
   \         |         |         |         |         /
    +--------+---------+---------+---------+--------+
                           |
                           v
                hawk-core-contracts
```

Important clarification:

- engines are at the same level
- `sight` and `inspect` are not below `eyrie` or below a separate execution tier
- Hawk decides when each engine is called
- engines do not coordinate each other directly

## Current problems this document resolves

Without a strict product map, a multi-repo workspace like this can drift into four
failure modes:

1. People treat support engines as separate products instead of Hawk capabilities.
2. Engines start importing each other for convenience.
3. Shared types leak from `hawk/internal` or ad hoc packages.
4. Company/platform repos such as `graycode-core` get confused with Hawk runtime dependencies.

The current architecture cleanup addressed the second and third risks with guardrails.
This document addresses the first and fourth by making the repo model explicit.

## Proposed steady-state architecture

The steady-state architecture for Hawk should be:

```text
top: integrations

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
    eyrie     yaad      tok       trace     sight    inspect   hawk APIs

bottom: shared contracts

                         hawk-core-contracts
```

## Repo classification

### 1. Primary product repo

#### `hawk`

Owns:

- CLI and command surface
- session and workflow orchestration
- task-semantic model intent and explicit user preferences
- tool execution policy
- approval model
- coordination of memory, context, tracing, review, and verification
- public product APIs used by SDKs and skills

This is the only primary end-user product in the Hawk ecosystem.

### 2. Support engine repos

These exist to power Hawk and should remain replaceable, testable, and isolated.

#### `eyrie`

Purpose:

- stable provider-engine host facade (`eyrie/engine`)
- credentials and provider authentication
- model discovery and capability catalog
- provider/deployment selection and infrastructure routing
- request/response normalization
- streaming, retry, timeout, fallback mechanics

Hawk composes this engine and owns every user-facing workflow around it. The
implemented boundary allows zero production imports of Eyrie's lower-level
packages; custom gateways are passed per Engine instance rather than installed
in global state.

#### `yaad`

Purpose:

- memory
- retrieval
- long-lived context persistence

#### `tok`

Purpose:

- token budgeting
- packing
- truncation
- context shaping

#### `trace`

Purpose:

- trace capture
- replay
- provenance
- audit visibility

#### `sight`

Purpose:

- review findings
- code quality/risk analysis
- normalized review output

#### `inspect`

Purpose:

- verification findings
- checks/assertions normalization
- pass/fail verification output

### 3. Shared foundation repo

#### `hawk-core-contracts`

Purpose:

- shared neutral contracts
- common findings and severity vocabulary
- events, tools, review, verify, and policy DTOs

This repo should remain:

- small
- stable
- implementation-light
- dependency-light

### 4. Consumer/extension repos

#### `hawk-sdk-go`

Purpose:

- Go integrations for Hawk public surfaces

#### `hawk-sdk-python`

Purpose:

- Python integrations for Hawk public surfaces

#### `hawk-community-skills`

Purpose:

- community skills
- recipes
- extension packs

These repos should consume Hawk surfaces, not bypass Hawk and talk to engines as a primary path.

### 5. Out-of-scope repo for Hawk runtime

#### `graycode-core`

Current meaning in this workspace:

- GrayCodeAI website/platform/company repo

For Hawk architecture decisions:

- it is not required in Hawk local runtime
- it is not part of Hawk’s core dependency graph
- it can later host company web, cloud, account, billing, or product control-plane concerns

That means `graycode-core` may matter to GrayCodeAI as a company, but it should not be treated as a Hawk engine.

## Current vs proposed

### Current workspace reality

```text
many repos in one workspace
        |
        +-- hawk is the primary product repo
        +-- support repos also exist as full sibling repos
        +-- hawk/external pins copies for integrated development and CI
        +-- graycode-core lives nearby but is not a Hawk runtime dependency
```

### Proposed steady-state interpretation

```text
one product
  -> hawk

six support engines
  -> eyrie
  -> yaad
  -> tok
  -> trace
  -> sight
  -> inspect

two shared foundations
  -> hawk-core-contracts
  -> hawk-mcpkit

three extension repos
  -> hawk-sdk-go
  -> hawk-sdk-python
  -> hawk-community-skills

separate company/platform repos
  -> graycode-core and future non-Hawk products
```

## Required dependency rules

### Allowed

```text
hawk -> eyrie
hawk -> yaad
hawk -> tok
hawk -> trace
hawk -> sight
hawk -> inspect
hawk -> hawk-core-contracts

engine -> hawk-core-contracts   # only when a true cross-repo contract is needed
engine -> hawk-mcpkit           # MCP server scaffolding (sight, inspect)
sdk -> hawk public API/contracts
skills -> hawk plugin/skill API
```

### Forbidden

```text
engine -> engine
sdk -> engine
skills -> engine
engine -> hawk/internal/*
engine -> graycode-core
hawk runtime -> graycode-core   # at compile time; opt-in, fail-open HTTP
                                # telemetry only, per adr/ADR-0001
```

## Why this is the right shape

This design gives the best balance of OSS clarity and industry-grade scale:

- product clarity: users understand they are adopting Hawk, not a bag of unrelated tools
- repo isolation: each engine can evolve, test, and release independently
- low coupling: peer engines do not create a dependency mesh
- better replacement path: a single engine can be rewritten or swapped without collapsing the system
- future cloud readiness: a hosted control plane can be added later without redesigning engine boundaries
- easier multi-product future: Hawk, Lark, and Gitant can later share company/platform layers without polluting Hawk runtime design

## Future-ready company view

The future GrayCodeAI portfolio can look like this without changing Hawk’s internal architecture:

```text
GrayCodeAI
├── Hawk      # coding agent product
├── Lark      # future product
├── Gitant    # future product
└── GrayCode platform/cloud
    ├── accounts
    ├── billing
    ├── hosted control plane
    ├── docs/web
    └── org/admin services
```

That is the right separation:

- product runtime architecture stays product-local
- company platform concerns stay above products, not inside support engines

## Final recommendation

For Hawk, keep exactly this product set:

- `hawk`
- `eyrie`
- `yaad`
- `tok`
- `trace`
- `sight`
- `inspect`
- `hawk-core-contracts`
- `hawk-sdk-go`
- `hawk-sdk-python`
- `hawk-community-skills`

Treat `graycode-core` as separate from Hawk runtime architecture.

Do not merge support engines into one another.
Do not let support engines depend on sibling engines.
Do not make SDKs or skills bypass Hawk.

That is the cleanest structure for OSS usability, production hardening, and future scale.
