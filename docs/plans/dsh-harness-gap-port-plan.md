# DSH gap port — close the remaining deepseek-harness catalogue gaps

Status: Phases 13–18 **delivered** on `feat/dsh-harness-port-p0-eventlog` (each package
verified: `gofmt`/`go vet`/`go test -race` green, `golang.org/x/image` added for
webp decode validation, OTel log modules pinned at v0.20.0 to keep core at
v1.44.0; full double-pass gate run below).

Anchors to the parent tracker:
[`docs/plans/dsh-harness-port-plan.md`](./dsh-harness-port-plan.md) declares the
protocol/session spine **functionally complete** (44/44 event types, projection,
persistence, tool/approval seams). This document tracks the **remaining upstream
feature-surface gaps** measured against `deepseek-ai/deepseek-harness`
`dsh-v0.1.0-rc.7` (Aug 17, 2026 clone) and their port status.

## Principles

Same as the parent plan:

- Port **protocols and invariants**, never Cordis.
- Everything Go-native: interfaces + a small registry + typed state, wired at
  the composition root.
- Keep new packages dependency-lean so the internal-layer guard
  (`scripts/check-internal-layer-imports.sh`) stays green.

## Gap inventory (from the comparison audit)

| # | DSH package(s) | Graycode gap | Port status |
| --- | --- | --- | --- |
| 13 | `identity/anonymous-user-id` | no per-home anonymous telemetry identity | **Delivered** |
| 14 | `web/web-search-deepseek`, `-exa`, `-perplexity` | web search only has Brave/SearXNG/DDG | **Delivered** |
| 15 | `jobs/jobs`, `jobs/jobs-local`, `jobs/tool-jobs` | no background job service | **Delivered** |
| 16 | `attachment/attachment` | no durable attachment seam | **Delivered** |
| 17 | `session/session-projection-cache` | projections recompute from seq 0 every time | **Delivered** |
| 18 | `session/session-telemetry-otel` | spans exist; no OTLP log-record export | **Delivered** |
| 19 | `session/session-title-*-llm` | deterministic `JournalTitle` only | deferred |
| 20 | `preset/agent-presets` | event + header metadata only, no standing mount | deferred |
| 21 | `session/session-persistence-sqlite`, `session-query/*sqlite`, `storage/storage-sqlite` | JSONL-only session persistence | deferred |
| 22 | `e2b/*` | no cloud Linux sandbox | deferred |
| 23 | `code-runtime/*` | no sandboxed model-written program execution | deferred |
| 24 | `sdk/client`, `sdk/protocol`, `sdk/server`, `python/sdk` | graycode exposes ACP server instead of DSH JSON-RPC SDK | deferred (keep ACP) |
| 25 | `terminal/*`, `tool-terminal` | no PTY terminal tool | deferred |
| 26 | `client/*`, `web/*`, `website`, `bundle/*-web-app` | web UI layer | out of scope (CLI/TUI) |

## Phase 13 — Anonymous user identity (`internal/identity`)

Port of DSH `identity/anonymous-user-id/src/index.ts`.

- `identity.Identity` — per-harness-home anonymous user id.
- Resolved once per process (memoized): `$GRAYCODE_HOME`-style home (`~/.graycode`),
  `.anonymous-user-id` file containing a bare random UUID.
- Never derived from hostname, network address, git remote, or env.
- Sync read/write; deleting the file mints a fresh identity.
- `SetHomeDir` test seam so tests never touch the real home.

## Phase 14 — Web search provider parity (`internal/tool`)

Port of DSH `web/web-search-deepseek`, `web/web-search-exa`,
`web/web-search-perplexity` providers into the existing `WebSearchTool` cascade
(Brave → SearXNG → DuckDuckGo → new providers).

- `web_search_deepseek.go` — Anthropic-compatible Messages API
  (`https://api.deepseek.com/anthropic/v1/messages`, model `deepseek-v4-flash`,
  native `web_search_20250305` tool, `max_uses` cap). Reuses `DEEPSEEK_API_KEY`
  but a separate base. Response parsing: `web_search_tool_result` blocks with
  `citations[]` → results.
- `web_search_exa.go` — Exa Search API (`https://api.exa.ai/search`,
  `EXA_API_KEY`, `contents.highlights` for snippets).
- `web_search_perplexity.go` — Perplexity chat completions
  (`https://api.perplexity.ai/chat/completions`, `PERPLEXITY_API_KEY`,
  model `sonar`, optional `search_recency_filter`; structured
  `search_results[]` → results, fallback `citations[]`).
- Availability gating identical to DSH: no API key → provider unavailable,
  cascade falls through.

## Phase 15 — Background jobs (`internal/jobs`)

Port of DSH `jobs/jobs` service + `jobs/jobs-local` registry semantics
(process-local), with a model-facing tool.

- `jobs.JobID` (`<kind>-N`), `jobs.Status`
  (`running|stopping|completed|killed|failed`), `jobs.Snapshot`,
  `jobs.Outcome` (`completed|killed|failed` + detail + output).
- `jobs.Registry` — `Start(kind, label, run)` with owner session scoping;
  `List(owner)`, `Read(id)`, `Wait(id)`, `Kill(id, reason)`, done listeners,
  owner cleanup on `ReleaseOwner`.
- Producer hooks: `cancel(reason)`, `done(outcome)`, `readOutput()`.
- `internal/tool/jobs_tool.go` — `JobsTool` (list / run / read / wait / kill)
  following the existing tool shape (Name/Aliases/Parameters/Execute).
- Output byte cap for model-facing notices.

## Phase 16 — Durable attachments (`internal/attachment`)

Port of DSH `attachment/attachment` version-one image path.

- `attachment.AttachmentID` (opaque, never a filesystem path).
- `attachment.ImageMediaType` (`image/png|jpeg|webp|gif`).
- `attachment.Ref` (ID, media type, bytes, width, height, optional name),
  `attachment.Limits` (maxImageBytes, maxImagesPerMessage,
  maxMessageImageBytes, maxImagePixels, mediaTypes), `attachment.SaveImage`,
  `attachment.Stored` (ref + data), `attachment.Store` interface +
  filesystem `Store` under the graycode home data dir.
- Validation: declared media type checked against decoded bytes; byte and
  pixel limits enforced; duplicate writes rejected; ID opaque.

## Phase 17 — Session projection cache (`internal/session/projcache.go`)

Port of DSH `session/session-projection-cache` semantics.

- `ProjCache` — durable per-session fold checkpoints, one record per session
  (`<seq> + ver + JSON state`).
- **Fold shortcut, never authority**: possibly stale (seq says how stale),
  never wrong; every write fail-soft; `ver` mismatch discards instead of
  migrating.
- `Load(sessionID)` → `(state, seq, fresh bool)`; `Save(sessionID, seq, state)`;
  `Discard(sessionID)`.
- JSON-file backend beside the session JSONL (DSH domain-data parity).

## Phase 18 — OTLP log-record export (`internal/observability/otellog`)

Port of DSH `session/session-telemetry-otel` backend semantics (the capture
coordinator stays out of scope — graycode has no Cordis bus; records reach the
backend via `Emit`/`EmitFeedback`).

- `Record{Channel, Time, Severity, Attributes, Body}` / `Sink` seam —
  channel (`ledger`/`ops`), three-level severity, minimal identity attributes,
  JSON-safe body (DSH `SessionTelemetryRecord`/`Sink`).
- `Mode` `FULL`/`FEEDBACK_ONLY`/`DISABLED` + `SharingStatus`; DISABLED
  constructs **no SDK state** and drops every record.
- Config validation at load: endpoint URL required + http(s) outside DISABLED,
  shutdown timeout positive (default 3s), batch size non-negative (DSH
  invariants).
- Severity mapping info→INFO(9)/warn→WARN(13)/error→ERROR(17); ledger and ops
  under separate instrumentation scopes; Resource carries
  service.name/version + `user.id` (via `internal/identity`, fail-soft).
- SDK pipeline composed as-is: `LoggerProvider` + batch processor +
  `otlploghttp` exporter (`WithEndpointURL` — full-URL semantics). OTel Go
  logs are still experimental (`otel/log`, `sdk/log`, `otlploghttp`
  **v0.20.0**, chosen to keep core at v1.44.0).
- Shutdown races the DSH deadline (default 3s); exporter-shutdown goroutine
  stays observed after the deadline; `Emit` is a non-blocking enqueue.
- `DefaultConfig()` mirrors oteltrace env conventions
  (`GRAYCODE_ENABLE_TELEMETRY=1` + `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`);
  wired in `cmd/graycode/main.go` and `cmd/daemon.go`.
- Tests: mode resolution, load-time validation, severity mapping, full/ops/
  ledger emission (in-memory exporter), feedback-only direct-drop,
  disabled-drop, idempotent shutdown, value conversion, attribute filtering.

## Gates

Each phase: `gofmt`/`go vet`/`go test` on the touched packages, then `make lint`
and `graycode verify` — run **twice** (the second pass re-runs the full touched
suite after any fixes). No direct commits to `main`.

### Verification record (2026-08-17, branch `feat/dsh-harness-port-p0-eventlog`)

Pass 1:

- `gofmt -l` (identity, tool, jobs, attachment, session, cmd/chat_tools.go): clean
- `go vet` (identity, tool, jobs, attachment, session, cmd): clean
- `go test -race` (identity, jobs, attachment): ok; `go test` (session, tool): ok
- `go build ./...`: ok
- `make lint`: **0 issues** (fixed pre-existing errcheck `check-type-assertions`
  debt + one unused helper in `internal/session/preparations.go`, and an S1000
  single-case select in `internal/jobs/jobs.go`)
- `graycode verify`: exit 0
- `scripts/check-internal-layer-imports.sh`: passed

Pass 2 (after lint fixes):

- `gofmt -l`: clean; `go vet`: clean; `go build ./...`: clean
- `go test -race` (identity, jobs, attachment): ok; `go test` (session, tool): ok
- `make lint`: 0 issues; `graycode verify`: exit 0

New dependency: `golang.org/x/image v0.45.0` (direct) — webp decode validation
for `internal/attachment` (stdlib has no webp decoder).

### Phase 18 verification (2026-08-17, branch `feat/dsh-harness-port-p0-eventlog`)

Pass 1:

- `gofmt -l` (cmd/graycode/main.go, cmd/daemon.go, internal/observability/otellog): clean
- `go vet` (otellog, cmd): clean
- `go test -race` (otellog): ok; `go build ./...`: ok
- `make lint`: **0 issues**
- `graycode verify`: exit 0
- `scripts/check-internal-layer-imports.sh`: passed

Pass 2 (uncached re-run, no fixes were needed):

- `gofmt -l`: clean; `go vet`: clean; `go build ./...`: clean
- `go test -count=1 -race` (otellog): ok
- `make lint`: 0 issues; `graycode verify`: exit 0

New dependencies (all direct): `go.opentelemetry.io/otel/log v0.20.0`,
`go.opentelemetry.io/otel/sdk/log v0.20.0`,
`go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.20.0` — the
log-signal v0.20.0 line pins exactly against core v1.44.0, so no OTel core
bump was required.
