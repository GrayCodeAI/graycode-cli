# ADR-0004: File-first canonical session history with a SQLite projection

- Status: Accepted
- Date: 2026-08-04
- Owners: Hawk maintainers

## Context

Hawk has several state-bearing components with different purposes:

- `PersistenceService` owns live in-memory transcript and context state.
- `internal/session` writes the durable JSONL session format and uses an
  external WAL for crash recovery.
- `SQLiteStore` provides a structured store with search and indexing support,
  but is not currently used by the active `Load`/`Save` path.
- Snapshots, conversation graphs, checkpoints, execution graphs, and graph
  journals preserve secondary state or projections.

Treating these as interchangeable authorities would create ambiguous recovery
semantics and make corruption or partial writes difficult to resolve.

## Decision

Hawk uses a file-first, projection-based persistence model:

1. **Runtime authority:** `PersistenceService` is authoritative only for the
   active in-memory session state.
2. **Durable authority:** JSONL is the canonical durable transcript and session
   format. The external WAL records recoverable writes around that format.
3. **Derived index:** SQLite may be used for searchable metadata, message
   indexes, and secondary queries. It is a rebuildable projection of JSONL, not
   an independent source of truth.
4. **Recovery rule:** A missing, stale, or corrupt SQLite projection must never
   prevent loading or resuming a valid JSONL session. WAL recovery failures
   remain explicit except for a not-found WAL.
5. **Secondary records:** Snapshots, checkpoints, conversation graphs,
   execution graphs, and graph journals are not substitutes for the canonical
   transcript. Each must document its own replay or rebuild behavior.
6. **Migration rule:** Activating SQLite indexing requires a separate
   implementation change with backfill, sequence/checksum validation, rebuild
   behavior, retention policy, and compatibility tests. No dual-authority or
   silent backend switch is allowed.

## Consequences

Positive:

- Existing JSONL sessions remain portable, inspectable, and backward
  compatible.
- Append-oriented WAL recovery has a clear role instead of competing with a
  database transaction log.
- SQLite can provide fast history search without making database corruption a
  session-loss event.
- Offline repair is straightforward: rebuild the projection from JSONL.

Trade-offs:

- Search indexes can be temporarily stale and require rebuild or backfill.
- Retention and compaction must preserve enough canonical history to rebuild
  the projection.
- A future hosted or multi-user deployment may require a different storage
  adapter, but it must preserve the same authority/projection contract.

## Required verification for SQLite activation

Before the dormant `SQLiteStore` becomes an active projection, add tests for:

- initial backfill from JSONL;
- idempotent rebuild after interruption;
- stale and corrupt index recovery;
- message ordering and tool-call fidelity;
- retention/compaction behavior;
- concurrent readers with one writer;
- successful resume when SQLite is unavailable.

