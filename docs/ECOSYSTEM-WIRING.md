# GrayCode ecosystem wiring

This document is the implementation contract for the 15 repositories in the
GrayCodeAI ecosystem. `ecosystem.yaml` is the canonical machine-readable
inventory; generated workspaces, boundary checks, release parity, and Owl's
repository catalog derive from it.

## Current state before this change

```mermaid
flowchart LR
  Lists[Hard-coded repository lists] --> CI[Per-repo CI]
  Lists --> Scripts[Workspace and release scripts]
  Lists --> Owl[Owl catalog]

  Hawk[Hawk product] --> Engines[Six engine repositories]
  Engines --> Eagle[Eagle contracts]
  Hawk --> Eagle

  HawkSpec[Hawk OpenAPI] -. manual snapshots .-> Sparrow
  HawkSpec -. manual snapshots .-> Robin
  HawkSpec -. no snapshot .-> Wren

  Browser[GrayCode browser] --> Mixed[One mixed Worker]
  Mixed --> Identity[(Identity tables)]
  Mixed --> Cloud[(Hawk Cloud tables)]
  Mixed -. fire-and-forget .-> Queue[Usage Queue]
```

The runtime engine integrations were mostly sound, but the surrounding wiring
could drift: repository names appeared independently in shell scripts and Owl,
SDK contracts were inconsistent, browser identity and the Hawk control plane
shared a Worker and D1 binding, and a successful usage write did not guarantee a
durable Queue-delivery intent.

## Implemented architecture

```mermaid
flowchart TB
  Manifest[hawk/ecosystem.yaml\ncanonical repo and contract inventory]
  Manifest --> Workspace[generated root go.work]
  Manifest --> Guards[boundary and release parity guards]
  Manifest --> OwlSnapshot[owl/ecosystem.json]

  subgraph Local[Local-first runtime]
    User[CLI / daemon user] --> Hawk[Hawk composition root]
    Hawk --> Eyrie[Eyrie facade]
    Hawk --> Harrier[Harrier / Harrier facade]
    Hawk --> Shrike[Shrike / Shrike facade]
    Hawk --> Swift[Swift / Swift facade]
    Hawk --> Kestrel[Kestrel / Kestrel facade]
    Hawk --> Merlin[Merlin / Merlin facade]
    Hawk --> Eagle[Eagle neutral contracts]
    Eyrie --> Eagle
    Harrier --> Eagle
    Shrike --> Eagle
    Swift --> Eagle
    Kestrel --> Eagle
    Merlin --> Eagle
  end

  Hawk --> Daemon[Hawk daemon API]
  Daemon --> OpenAPI[api/openapi.yaml]
  OpenAPI --> Sparrow[Sparrow Go SDK snapshot]
  OpenAPI --> Robin[Robin Python SDK snapshot]
  OpenAPI --> Wren[Wren TypeScript SDK snapshot]

  subgraph Hosted[Optional hosted plane]
    Browser[GrayCode browser] --> BFF[GrayCode identity/BFF Worker]
    BFF --> Identity[(Identity D1)]
    BFF -->|private typed Service Binding| Cloud[Hawk Cloud Worker]
    Cloud --> Control[(Control-plane D1)]
    Cloud -->|same D1 transaction| Outbox[(Usage outbox)]
    Outbox -->|immediate + scheduled retry| Queue[Cloudflare Queue]
    Queue --> Rollups[(Idempotent rollups)]
    Queue --> Archive[(R2 archive)]
  end

  Hawk -. explicit opt-in; fail-open .-> Cloud
```

## Ownership rules

| Boundary | Owner | Rule |
|---|---|---|
| Product orchestration | Hawk | Engines never orchestrate Hawk or one another. |
| Provider runtime | Eyrie | Hawk imports its supported `engine` facade. |
| Shared data contracts | Eagle | Neutral types only; no product behavior. |
| Portable local graph | Source engine/Hawk | Emit bounded `hawk.graph/v1` facts without raw secrets or prompts. |
| Daemon HTTP API | Hawk | `hawk/api/openapi.yaml` is authoritative. |
| SDK endpoint support | Each SDK | Exact contract snapshot plus an explicit decision for every path. |
| Browser identity | GrayCode BFF | Users, sessions, email, API keys, and UI activity remain in identity D1. |
| Hosted control plane | Hawk Cloud | Organizations, projects, devices, usage, billing, graph ledger, and audit. |
| Cloud delivery | D1 outbox + Queue | Business state and delivery intent commit together; consumers are idempotent. |
| Architecture discovery | Owl | Generated projection of the canonical manifest, never a second inventory. |

## Repository identity

Bird codenames are repository and Go module identities. Product names remain
the user-facing labels. In particular, `harrier` is Harrier, `shrike` is Shrike,
`swift` is Swift, `kestrel` is Kestrel, and `merlin` is Merlin. Scripts and CI
must use `directory`/`github_repo`; UI copy may use `product_name`.

## Contract and event flow

1. Engine packages expose only the facade declared in `ecosystem.yaml`; the
   manifest validator rejects a missing or out-of-module facade.
2. Eagle commit `9d358dde4ad8` is the cross-repository contract revision, pinned
   through its reachable Go pseudo-version until the next semver tag is published;
   the parity gate checks every declared consumer.
3. Local graph projections use `hawk.graph/v1`. Hawk Cloud independently
   validates and privacy-normalizes them into `graycode-cloud.graph/v1`.
4. Usage ingestion commits its idempotency claim, budget counter, event,
   billing ledger row, daily activity projection, and `usage.recorded.v1`
   outbox record in one ordered D1 batch.
5. Queue publication is at-least-once. A successful send marks the outbox row;
   failures back off and the cron drain retries them. Queue consumers dedupe on
   `eventId`, so a send/mark race cannot double-count.

## Release and development workflow

- Run `make workspace` in Hawk to regenerate the parent `go.work`.
- Run `make boundaries` to validate the inventory, module isolation, Eagle
  parity, facade locations, and support-repository coupling.
- Root `go.mod` files never contain local `replace` directives. Local sibling
  substitutions exist only in the generated, uncommitted parent workspace.
- SDK CI checks its snapshot byte-for-byte against Hawk and separately checks
  that every OpenAPI path has a support decision.
- Run `owl/scripts/sync-ecosystem.sh` after changing the canonical inventory;
  Owl CI rejects drift.
- Standalone/module-mode tests remain the release truth; workspace tests are an
  additional integration pass, not a substitute.

### Coordinated Go publication gate

The compatible Eagle-migrated Eyrie source is currently ahead of Eyrie's
published `origin/main`. Do not merge or release Hawk against the older Eyrie
pseudo-version: it still exposes the retired `hawk-core-contracts` types and is
not type-compatible with Hawk's Eagle boundary.

1. Merge and publish the Eyrie ecosystem-wiring branch first.
2. Resolve that final remote commit to its canonical Go pseudo-version with
   `go list -m github.com/GrayCodeAI/eyrie@<commit>`.
3. Update Hawk's Eyrie requirement, run `GOWORK=off go mod tidy`, and remove any
   transition excludes no longer required by the published engine graphs.
4. Require Hawk's `public-modules` and `release-parity` CI jobs to pass before
   merging or tagging Hawk.

This publication is intentionally not performed by the source implementation:
remote branch merges and tags are externally visible release operations.

## Deployment sequence for the Worker split

1. Create or automatically provision the `graycode-identity` D1 database and
   confirm its binding in `apps/bff/wrangler.jsonc`.
2. Export the existing identity tables from the legacy mixed database and
   import them into the identity database; verify row counts and login flows.
3. Deploy `graycode-cloud` with the named `HawkCloudService` entrypoint and
   apply migration `0023_usage_outbox.sql`.
4. Deploy `graycode-bff` with its D1 binding and private service binding.
5. Point the web frontend API hostname at the BFF. Hawk/CLI device start and
   poll traffic continues to target Hawk Cloud.
6. After a rollback window, remove the legacy identity tables from the old
   cloud D1 using a separately reviewed data-retirement migration.

The database move is intentionally an operator-controlled deployment step:
creating Cloudflare resources or deleting the legacy copy is not safe to infer
from source-code changes alone.
