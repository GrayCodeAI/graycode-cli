# Hawk–Eyrie Engine Migration

## Target

```text
User ──► Hawk product/agent ──► eyrie/engine ──► model providers
```

The implemented split is:

```text
Hawk — product face                         Eyrie — provider engine

CLI / TUI / SDK entrypoints
  ├─ /config and model picker ───────────► engine control plane
  │                                         ├─ OS credential store
  │                                         ├─ catalog/discovery
  │                                         └─ provider state + routing policy
  └─ conversation + coding agent
       ├─ history / WAL / resume
       ├─ tools / permissions / policy
       └─ Hawk ChatClient port ───────────► engine generate/stream
                                             ├─ capability/model resolution
                                             ├─ deployment routing + resilience
                                             └─ normalized events/usage
```

Hawk remains authoritative for the coding-agent loop, tools, permissions,
project context, product memory, conversation history, WAL, checkpoints, and
resume/replay. Eyrie owns credentials, catalog discovery, capability matching,
provider/deployment routing, resilience, normalized streaming, usage, cost,
health, and provider telemetry.

## Source workflow

Standalone `eyrie/` is developed and tested first. Hawk then advances
`external/eyrie` to the exact Eyrie commit and verifies the clean submodule
checkout before integrating it. Published Hawk builds continue to use a tagged
Eyrie module version; `go.work` pins the submodule for local integration.

Upgrade order is deliberate:

1. implement and test the facade change in standalone Eyrie
2. create a reviewable Eyrie commit and ensure the revision is reachable
3. advance Hawk's `external/eyrie` gitlink to that exact commit
4. update Hawk's `go.mod` version after the Eyrie revision is published
5. run `go work sync`, then verify Hawk once through the submodule and once with
   `GOWORK=off`

Do not copy Eyrie source into Hawk or let a Hawk change depend on an unpinned
standalone checkout.

## Migration rules

1. New host-boundary code uses `github.com/GrayCodeAI/eyrie/engine`.
2. Hawk production code imports no Eyrie package below `eyrie/engine`.
3. Hawk expresses requirements and intent; Eyrie resolves infrastructure.
4. An exact user model does not silently fall back.
5. Eyrie emits tool requests; Hawk authorizes and executes tools.
6. Hawk has one authoritative product conversation store.
7. Secrets stay in Eyrie's credential store and never enter tool environments.
8. Custom gateways are Engine-instance configuration, not process-global state.
9. Hawk-owned persisted and CLI schemas do not mirror Eyrie DTOs implicitly.

## Current slice

- Eyrie provides the versioned host facade, provider-neutral DTOs, typed errors,
  capability selection, normalized pull streaming, credential service, catalog
  snapshots, injected credential/state paths, and explicit catalog-backed
  deployment construction.
- Hawk's credential-save and production agent-chat paths now enter through
  `eyrie/engine` via a Hawk-owned `ChatClient` adapter.
- Provider secrets are absent from Hawk's `Session`, `ChatService`, sub-session,
  reattachment, and client-port surfaces. Native provider compaction uses
  Eyrie's injected credential store and engine facade; Hawk receives only the
  normalized summary and remains responsible for conversation mutation.
- Eyrie's control-plane facade now supplies credential resolution, safe masked
  status, provider choices, and gateway configuration rows using its injected
  credential store and state paths. Hawk owns their TUI/CLI presentation.
- Historical provider-state credentials are imported into the Engine's secret
  store before an atomic sanitized rewrite. Every later provider-state write
  uses the same sanitizer; Hawk protects the resolved
  `EYRIE_CONFIG_DIR/provider.json` path from agent file and Bash access.
- Effective provider/model selection is an `eyrie/engine.Selection` contract;
  Hawk's session factory, startup, live transport rebuild, and multi-agent
  workers no longer depend on Eyrie's lower-level runtime selection DTOs.
- The model picker consumes display labels, ownership, serving gateway,
  context, capabilities, pricing and price certainty from `engine.Model`; it
  no longer reads or formats Eyrie's compiled catalog directly.
- `hawk models list --json` projects those DTOs into a stable Hawk-owned schema;
  `--raw` exposes provider-native live metadata when present without coupling
  automation to the engine DTO.
- Hawk retains task classification, workflow roles, cascade decisions and
  health thresholds. Model lookup, aliases, provider ownership, defaults,
  relative cost classes and preferred candidates now come from Eyrie's
  host-neutral model-policy facade.
- The facade preserves advanced generation options and owns continuation;
  Hawk's compatibility retry/rate-limit wrapper is bypassed for facade clients
  so resilience is applied exactly once.
- Catalog administration, setup/diagnostics, review bridges, session creation,
  parallel agents, custom gateways, and inline tool-call normalization all use
  the engine boundary. The zero-exception rule is enforced by shell and AST
  guards.
- Hawk supplies effective custom-provider settings in
  `engine.Options.CustomGateways`; each Engine snapshots its own gateway set so
  sessions and tests cannot leak configuration through globals.
- `hawk preflight` describes local readiness. A provider-scoped live model
  fetch or `/config` validation is the optional live-verification step; local
  readiness alone is not a remote authentication claim.
- Hawk's runtime conversation DTOs remain product-owned in `internal/types`;
  the anti-corruption adapter in `internal/engine` translates them directly to
  stable engine DTOs without importing a lower Eyrie transport package.
- Hawk now owns its persistent conversation graph under `internal/session`;
  production sessions no longer mix Eyrie's generic DAG with Hawk WAL/session
  persistence.

## Completed removal gates

Production catalog/setup/runtime/client compatibility imports are removed.
Lower Eyrie packages may appear only in tests that construct Eyrie-owned
fixtures. The provider circuit breaker, session API-key map, direct client
adapter, and mixed Eyrie DAG product path are also removed. Session file
readers remain backward-compatible for at least one release cycle.

## Verification and release status

See `verification-status-2026-07-13.md` for the current evidence ledger and
remaining blockers. In particular, the audited workspace's committed Eyrie
Gitlink, checked-out submodule and `go.mod` module revision do not match. The
source boundary is implemented locally, but that mismatch must be resolved and
verified in both workspace and `GOWORK=off` builds before the migration can be
called release-complete.
