# Hawk Architecture Baseline

**Status:** Phase 0 baseline
**Date:** 2026-08-04
**Baseline commit:** `69ce83f55f9098623e5a891e8c52be636db89c7c`

This document records the architecture that exists in the Hawk repository at
the beginning of the architecture improvement program. It separates current
implementation facts from the intended ecosystem design. It is not a product
quality score and does not claim that the migration is complete.

## Authority and terminology

Use the documents in this order when statements conflict:

1. This document for the dated implementation baseline and migration status.
2. `hawk-current-vs-proposed.md` for the ecosystem repository map.
3. `hawk-product-architecture.md` for ownership and runtime responsibilities.
4. `hawk-dependency-rules.md` for allowed and forbidden dependency edges.
5. `spec.md` for behavioral requirements and agent-loop semantics.

Hawk is a Go repository and workspace entry point in a multi-repository
ecosystem. The `external/` directory contains pinned support repositories for
reproducible integration; it does not make the support engines one monorepo.

## Target product graph

```text
users / SDKs / skills / daemon clients
                |
                v
              hawk
       /        |        \
    eyrie     yaad       tok
    trace     sight    inspect
                |
                v
       hawk-core-contracts
```

The graph is intentionally directional:

- Hawk owns user-facing orchestration, sessions, tools, permissions,
  composition, and public product surfaces.
- Eyrie owns provider protocols, routing, credentials, catalogs, and provider
  execution behind `eyrie/engine`.
- Yaad, Tok, Trace, Sight, and Inspect are support engines and must not import
  Hawk internals or one another.
- Core contracts contain stable cross-repository vocabulary and DTOs, not
  runtime orchestration.
- SDKs and skills consume Hawk surfaces rather than support-engine internals.

## Current implementation state

### Complete or enforced

- Hawk production code uses Eyrie through the `eyrie/engine` facade.
- Sight and Inspect are integrated through Hawk bridge packages.
- Support-engine sibling imports and imports of Hawk internals are guarded.
- The AST/package-graph guard reports production boundary violations with
  file/line diagnostics across Hawk and available support repositories.
- Persisted tool, review, verification, event, and policy contracts use the
  implemented portions of `hawk-core-contracts`.
- Native-compaction capability contracts use `hawk-core-contracts/llm`; Eyrie
  request translation remains inside `internal/provider/gateway`, keeping the
  engine layer independent of the provider adapter package for this path.
- Container-required state and its executor are owned by `ToolService` and
  read through synchronized snapshots, including asynchronous TUI retry.
- `GraphAwareBudget` reads Yaad through `YaadBridge`; its graph-budget path no
  longer imports Yaad engine or storage implementation types directly.
- `CodeMemoryLinker` also routes node search, edge creation, and file-anchor
  persistence through `YaadBridge`; remaining direct Yaad users are isolated
  to the other memory workflow slices awaiting migration.
- The local boundary suite, full Go tests, and `go vet` pass at this baseline.

### Transitional

- `internal/engine.Session` still contains service fields and remaining legacy
  state. The service graph is authoritative in the migrated paths, but the
  decomposition is not complete.
- `Session.Cost` remains a public compatibility field because existing callers
  assign it directly. New code must use `CostValue()`; removing the field is a
  versioned API change, not a safe internal extraction.
- `internal/engine` remains a large compatibility and orchestration package.
  At this baseline its top-level production files contain approximately
  19,253 lines, its top-level tests approximately 11,855 lines, and the
  subtree contains compatibility alias/re-export files.
- Hawk's Yaad and Tok implementation imports are now consolidated behind
  `YaadBridge` and `internal/token` for the migrated production paths. The
  remaining direct Yaad users are isolated workflow or test integrations;
  replaceability is improved, but still not equivalent to the Eyrie boundary.
- `PersistenceService` is the in-memory runtime owner for transcript/context
  state and checkpoint metadata. The active durable session path remains
  `internal/session` JSONL plus the external file WAL used for crash recovery.
  `SQLiteStore` is implemented but dormant: it is not the active `Load`/`Save`
  backend. Workspace snapshots, conversation graphs, graph journals, and
  execution graphs are secondary records or projections, not the canonical
  durable transcript. An ADR is still required to define whether JSONL/WAL
  remains authoritative or SQLite becomes authoritative, including migration,
  retention, and recovery semantics.
- CLI, daemon, and other entry points share substantial construction and
  orchestration responsibilities instead of depending on one explicit
  application composition root.
- Non-interactive entry points now share `cmd.newConfiguredHawkSession`; the
  interactive TUI intentionally retains a lightweight startup path followed by
  deferred heavy configuration to protect first-frame latency.

## Architecture decisions for the improvement program

### ADR-B01 — Keep the multi-repository ecosystem

Do not collapse the support engines into Hawk or force a shared release cycle.
The `hawk-eco` workspace and Hawk's pinned `external/` modules provide local
integration without removing independent ownership and release boundaries.

### ADR-B02 — Preserve the Eyrie boundary

Provider implementation, catalog metadata, credential mapping, and protocol
adapters remain owned by Eyrie. Hawk may own product policy and user-facing
selection, but production provider access remains through `eyrie/engine`.

### ADR-B03 — Complete internal consolidation before adding new seams

The next architecture work prioritizes Session decomposition, composition-root
centralization, and persistence ownership. New engines or broad contract
packages should not be added until the current seams are explicit.

### ADR-B04 — Add facades selectively

Yaad, Tok, and Trace require a facade decision based on actual replacement and
release needs. A facade is justified when it isolates Hawk from implementation
types or enables independent upgrades; it is not justified merely to increase
the number of packages.

### ADR-B05 — No subjective architecture score is a release criterion

Documents may compare capabilities and trade-offs, but unsupported scores such
as “9.2/10” or “10/10” are not architecture evidence. Architecture readiness
must be assessed using dependency checks, tests, migration completion, and
operational guarantees.

## Non-goals

This program does not aim to:

- rewrite the agent loop from scratch;
- merge all engines into one repository;
- move every runtime or persistence type into `hawk-core-contracts`;
- make every engine use identical integration depth;
- add IDE parity before the internal architecture is stable;
- treat passing tests as proof that migration work is complete.

## Baseline verification

The following checks passed against the baseline commit:

```text
make boundaries
go test ./internal/testaudit/... -count=1
go test ./... -count=1 -timeout=120s
go vet ./...
```

The GitNexus workspace index also reports the baseline commit as up to date.
Architecture work must still run impact analysis before changing code symbols
and change-scope detection before committing.

## Next phase

Phase 1 adds AST/package-graph dependency checks. Phase 2 continues the safe
Session migration using the boundaries documented here, with `Session.Cost`
explicitly retained as a compatibility exception. Lazy persistence
initialization, cost snapshots, and WAL recovery error reporting are now
synchronized and tested. Phase 3 has explicit non-interactive and interactive
startup composition boundaries, with heavy TUI configuration remaining
deferred for first-frame latency. Phase 4 has consolidated the migrated Yaad
and Tok implementation imports behind narrow Hawk-owned facades. The next
decision is the persistence ADR: document and enforce one durable authority,
then define the migration and recovery contract before introducing additional
storage backends.

## Current branch follow-up

The architecture is strong but transitional, not perfect. The highest-value
remaining risk is persistence authority: several storage and observability
mechanisms exist, but only JSONL plus the external WAL currently define durable
session recovery. No code should silently switch the active backend until the
persistence ADR is approved and covered by compatibility and recovery tests.
