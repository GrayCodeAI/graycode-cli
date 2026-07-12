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
- The facade preserves advanced generation options and owns continuation;
  Hawk's compatibility retry/rate-limit wrapper is bypassed for facade clients
  so resilience is applied exactly once.
- Catalog administration and review-only bridges retain compatibility imports
  while their facade contracts are completed.

## Removal gates

Hawk's mirrored transport types, provider circuit breaker, legacy API-key map,
and mixed Eyrie DAG usage may be removed only after equivalent facade contract
and end-to-end tests pass. Session file readers remain backward-compatible for
at least one release cycle.
