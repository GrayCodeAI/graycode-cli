# graycode-eco ecosystem architecture

This document records the target repository and dependency architecture for the
fourteen repositories audited in July 2026. It distinguishes source integration,
published dependencies, runtime calls, and product ownership; those relationships
must not be treated as interchangeable.

## Repository boundary

| Repository | Visibility target | Responsibility | Dependency rule |
| --- | --- | --- | --- |
| `hawk` | Public | CLI, local daemon, orchestration, policy and engine composition | Integration root; resolves the seven engine repos as workspace siblings and published Go modules |
| `eyrie` | Public | Provider engine: credentials, catalog, routing, transport and normalized streaming | Hawk consumes only its `eyrie/engine` host facade; no Hawk import |
| `hawk-core-contracts` | Public | Shared Go domain contracts | Leaf contract module; must not import product implementations |
| `tok` | Public | Token analysis and optimization engine | Independently releasable library and MCP server |
| `trace` | Public | Trace and observability engine | Embeds only Hawk's supported CLI root boundary when built with Hawk |
| `yaad` | Public | Memory and knowledge engine | Independently releasable library; SSE is implemented, gRPC remains planned |
| `sight` | Public | Source/diff review engine | Source-review boundary; does not own deployed-target inspection |
| `inspect` | Public | Live/deployed target inspection | Runtime inspection boundary; does not own source diff review |
| `hawk-mcpkit` | Public | Shared MCP primitives | Intentionally narrow reuse by compatible MCP servers, currently Sight and Inspect |
| `hawk-sdk-go` | Public | Go client SDK for the Hawk daemon API | Contract-checked against `hawk/api/openapi.yaml` |
| `hawk-sdk-python` | Public | Python client SDK for the Hawk daemon API | Contract-checked against `hawk/api/openapi.yaml` |
| `hawk-community-skills` | Public | Community-authored skill definitions and validation | Content/extension plane; no privileged product secrets |
| `hawk-cloud` | Private | Hosted control plane, verified usage ledger, credits, billing, enterprise policy and delivery metadata | Sole authority for Hawk billing/metering data; exposes a versioned OpenAPI contract |
| `graycode-core` | Private | GrayCode product UI and backend-for-frontend | Calls Hawk Cloud through a service binding and contract-checked adapter; never owns the Hawk ledger |

The visibility column is the recommended governance target. Package-level
`private` flags prevent accidental publication but do not configure GitHub
repository visibility; repository settings must be verified separately.

## Current versus target

| Concern | Audited current state | Target state |
| --- | --- | --- |
| Local engine integration | Seven Hawk submodules previously existed, but module and checkout versions could drift | Hawk's `go.work` resolves workspace siblings (`../<repo>`) and `go.mod` versions identify the same public commits; CI verifies both workspace and module modes |
| Public releases | Release setup could fall back to a branch head | A missing pinned commit fails the release; no branch fallback |
| Hawk daemon API | Hawk, both SDKs and their snapshots could drift | `hawk/api/openapi.yaml` is authoritative; SDK CI compares the exact contract and tests supported operations |
| Hosted API | Cloud routes and GrayCode calls were manually coupled | `hawk-cloud/contracts/openapi.yaml` is authoritative; route and BFF reference tests reject undocumented paths |
| Billing trust | Client-reported estimated cost could enter billing calculations | Clients report token dimensions and optional estimates; Hawk Cloud prices them with a versioned server catalog and bills only verified cost |
| Product data | GrayCode contained similarly named usage data | GrayCode telemetry remains product-local; Hawk Cloud is the sole authoritative usage, budget, credit and billing ledger |
| Cloud deployment | Environment bindings and generated types could drift | JSONC configuration, generated binding types, observability and dry-run production bundles are checked in CI |
| Authorization | Coverage varied by route and some lookups preceded authentication | Authentication occurs before protected resource lookup; every route family has positive/negative matrix coverage |
| Module boundaries | Large route files and undocumented engine overlap obscured ownership | Domain-specific BFF/cloud routes, Hawk import guards and explicit Sight/Inspect/Trace/Yaad boundaries |

## Dependency and runtime topology

```text
community skills ────────────────> Hawk extension/content plane

hawk-sdk-go ───────┐
hawk-sdk-python ───┴─ HTTP ─────> Hawk local daemon
                                      │
hawk (integration root) ──────────────┼─ eyrie/engine
  workspace ../siblings + Go modules ──┼─ core-contracts
                                      ├─ tok
                                      ├─ trace
                                      ├─ yaad
                                      ├─ sight
                                      └─ inspect
                                           └─ compatible MCP helpers: hawk-mcpkit

graycode-core UI -> GrayCode BFF -> Cloudflare service binding -> hawk-cloud
                                                               ├─ D1 ledger/control data
                                                               ├─ Queue usage events
                                                               └─ R2 exports
```

Source-level arrows are allowed only in the displayed direction. Hawk Cloud
must not import GrayCode product code, engines must not import Hawk orchestration,
and public repositories must not depend on private repositories.

## Workspace policy

Hawk resolves the seven engine repos as independent sibling checkouts via its
`go.work`: `eyrie`, `hawk-core-contracts`, `inspect`, `sight`, `tok`, `trace`,
and `yaad`. They do not replace module releases. Downstream Go consumers use
published module versions; Hawk CI tests both the workspace and `GOWORK=off`
public-module graph. `hawk-mcpkit`, SDKs, community skills, Hawk Cloud and
GrayCode are not workspace siblings for local engine integration because they
are not linked into the engine dependency graph.

## Repository count decision

No fifteenth product repository is required now. Shared contracts already have
appropriate homes: daemon contracts in `hawk`, hosted contracts in `hawk-cloud`,
and shared Go types in `hawk-core-contracts`. Create another repository only when
an independently owned artifact has a distinct release cadence and at least two
real consumers; do not create an infrastructure, documentation, or generic
contracts repository merely to move files across a repository boundary.

## Architecture invariants

1. Public code never requires access to either private repository to build or test.
2. Every runtime route is represented by its owning OpenAPI document.
3. Client-controlled monetary values never affect budgets, credits, invoices or the verified ledger.
4. Protected resource existence is not disclosed before authentication and authorization.
5. Every engine commit used by Hawk is publicly reachable and matches the corresponding module version.
6. Production Cloudflare bindings are explicit, typed and verified with a dry-run bundle.
7. Cross-repository contract drift and forbidden imports fail CI.
8. Hawk production code reaches provider credentials, catalogs and transport
   only through `eyrie/engine`; Hawk-owned conversation and CLI schemas remain
   separate.
