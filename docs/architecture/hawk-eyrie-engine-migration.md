# Hawk–Eyrie Engine Migration

## Target

```text
User ──► Hawk product/agent ──► eyrie/engine ──► model providers
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

## Migration rules

1. New host-boundary code uses `github.com/GrayCodeAI/eyrie/engine`.
2. Existing lower-level imports remain temporary compatibility exceptions.
3. Hawk expresses requirements and intent; Eyrie resolves infrastructure.
4. An exact user model does not silently fall back.
5. Eyrie emits tool requests; Hawk authorizes and executes tools.
6. Hawk has one authoritative product conversation store.
7. Secrets stay in Eyrie's credential store and never enter tool environments.

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
- Effective provider/model selection is an `eyrie/engine.Selection` contract;
  Hawk's session factory, startup, live transport rebuild, and multi-agent
  workers no longer depend on Eyrie's lower-level runtime selection DTOs.
- The model picker consumes display labels, ownership, serving gateway,
  context, capabilities, pricing and price certainty from `engine.Model`; it
  no longer reads or formats Eyrie's compiled catalog directly.
- Hawk retains task classification, workflow roles, cascade decisions and
  health thresholds. Model lookup, aliases, provider ownership, defaults,
  relative cost classes and preferred candidates now come from Eyrie's
  host-neutral model-policy facade.
- The facade preserves advanced generation options and owns continuation;
  Hawk's compatibility retry/rate-limit wrapper is bypassed for facade clients
  so resilience is applied exactly once.
- Catalog administration and review-only bridges retain compatibility imports
  while their facade contracts are completed.
- Hawk now owns its persistent conversation graph under `internal/session`;
  production sessions no longer mix Eyrie's generic DAG with Hawk WAL/session
  persistence.

## Removal gates

Hawk's mirrored transport types and remaining catalog/setup compatibility
imports may be removed only after equivalent facade contracts and end-to-end
tests pass. The provider circuit breaker, session API-key map, and mixed Eyrie
DAG product path have already been removed. Session file readers remain
backward-compatible for at least one release cycle.
