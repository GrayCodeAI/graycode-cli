# Hawk Execution Graph

## Status

The first read-only execution-graph projection is implemented. It does not
replace Hawk's scheduler or move runtime ownership into a graph database.

```text
runtime owners
  session persistence
  structured task store
  background task registry
  permission engine
  verification bridges
  graph observation journal
  Harrier retrieval + Hawk code index
  Merlin + Kestrel quality projections
  Shrike runtime projection
  Eyrie operations projection
  Swift checkpoints
        |
        v
internal/executiongraph
        |
        v
eagle/graph
        |
        +--> hawk graph export
        |
        +--> authenticated daemon session graph API
        |
        +--> privacy-normalized, explicit Hawk Cloud sync
```

## Ownership

| Data | Source of truth | Graph behavior |
|---|---|---|
| Hawk sessions and messages | `internal/session` | Read-only projection |
| Structured tasks and dependencies | `internal/tool` task store | Read-only projection |
| Agent, shell, and monitor tasks | `internal/taskruntime` | Read-only projection |
| Permission verdicts | permission subsystem | Automatically summarized after tool permission and approval gates |
| Verification reports | verification engines/bridges | Built-in plan verification is automatically summarized; other engines can supply typed observations |
| Retrieved memory context | Harrier | Exact metadata-only retrieved subgraph is journaled and linked to the Hawk session |
| Retrieved code context | Hawk code index in Harrier storage | Metadata-only code-chunk knowledge nodes are journaled |
| Website audit quality | Merlin | Bounded report/finding subgraph and neutral verification summary are journaled when the caller supplies a session identity |
| Code review quality | Kestrel | Bounded review/finding subgraph and neutral verification summary are journaled when the caller supplies a session identity |
| Context compression operations | Shrike | Compression statistics are automatically captured during persisted-session context compaction |
| Response redaction quality | Shrike | Aggregate Shrike-only secret matches are automatically captured after persisted-session response redaction |
| Token usage and budget decisions | Shrike | Deduplicated provider usage updates Shrike's tracker; usage and next-turn budget decisions are automatically captured |
| Model routing and generation usage | Eyrie | Route and normalized usage operations are automatically captured after each persisted-session model turn |
| Observation history | `internal/graphjournal` | Append-only, privacy-safe runtime evidence |
| Swift sessions and checkpoints | Swift | Exact start-time metadata correlation plus stable external references |

The graph package never mutates these owners and never schedules work.

## Validated scheduling view

Hawk's structured task store now validates its blocking dependency graph before
answering `TaskList {"action":"ready"}`. The validator:

- caps the view at 1,000 tasks and 10,000 unique blocking edges;
- rejects missing blockers, self-dependencies, invalid task states, and cycles;
- ignores `related` and `parent-child` relationships for scheduling;
- returns stable, sorted topological waves plus the currently runnable pending
  tasks;
- fails closed when legacy callers request ready work from an invalid graph.

This is graph-driven readiness selection with an opt-in execution bridge. The
default agent/tool loop remains authoritative, but `hawk mission --from-tasks`
can now consume the validated waves, execute each wave with bounded parallel
workers, and persist a deterministic join record before any later wave starts.
If a wave fails, later waves remain pending rather than being speculatively
dispatched.
The same execution bridge is available to library callers through
`Mission.RunStaged(..., WithExecutionWaves(waves))`, so staged PRD,
verification, and fix pipelines can retain graph ordering without depending on
the CLI or TaskStore.

Graph-driven execution keeps the existing operational controls:

- the mission worker cap bounds fan-out;
- worktrees still provide per-feature isolation;
- mission persistence records each wave join durably;
- a portable `mission-graph.json` artifact is rewritten alongside `mission.json`
  so graph consumers can merlin mission state without reading mutable runtime
  structs;
- later waves are cancellation- and failure-sensitive;
- downstream tasks are never unlocked by a failed blocker.

## Portable topology

Nodes:

- `hawk/session/<id>` — persisted Hawk session;
- `hawk/task-request/<session>/<n>` — user task request, content represented by
  SHA-256 only;
- `hawk/task/<id>` — structured task;
- `hawk/runtime-task/<id>` — background agent, shell, or monitor task;
- `hawk/tool-call/<session>/<tool-use-id>` — tool invocation metadata;
- `hawk/policy/<id>` — permission verdict;
- `hawk/verification/<id>` — neutral verification result;
- `harrier/memory/<id>` — retrieved memory knowledge;
- `hawk/code-chunk/<digest>` — retrieved code-index knowledge;
- `merlin/report/<digest>` and `merlin/finding/<digest>` — website audit
  quality facts;
- `kestrel/review/<digest>` and `kestrel/finding/<digest>` — code-review quality
  facts;
- `shrike/compression/<digest>`, `shrike/usage/<digest>`, `shrike/budget/<digest>`, and
  `shrike/redaction/<digest>` — operations, policy, and quality facts;
- `eyrie/route/<digest>` and `eyrie/generation/<digest>` — model route and
  normalized generation operations;
- `swift/checkpoint/<id>` — externally owned Swift checkpoint reference.

Edges:

- `contains` — session/task hierarchy and task-to-tool containment;
- `depends_on` — blocking task dependency;
- `references` — related tasks and Hawk-to-Swift checkpoint linkage;
- `governed_by` — execution subject to policy verdict;
- `validated_by` — execution subject to verification result.
- imported Merlin `contains` edges — report-to-finding hierarchy.

All nodes, edges, and events pass the shared contract validators. Edges whose
source or target node is absent are rejected.

## Privacy and bounded payloads

The export deliberately excludes:

- prompt and response bodies;
- tool arguments and tool result content;
- background task prompts and output;
- policy reason text;
- verification targets, messages, fixes, and evidence;
- absolute working-directory paths.

Where correlation is useful, sensitive values are represented by SHA-256
digests. Every node declares `data_classification=metadata_only`.

Hawk writes runtime observations to a per-session append-only JSONL journal.
The journal is stored with mode `0600` under Hawk's state directory and contains
only verdict metadata, aggregate verification counts, and SHA-256 digests. A
journal failure is logged and never changes a permission, approval, or tool
result.

Mission-mode graph execution persists a separate `mission-graph.json` artifact
beside `mission.json` in the mission directory. The artifact uses the same
portable `hawk.graph/v1` envelope, but it is mission-scoped rather than
session-scoped: mission node, feature nodes, and wave-join operations nodes are
rewritten after each mission run or wave join using only metadata and SHA-256
digests.

## CLI

```bash
hawk graph export [session-id]
  --repository <scope>
  --swift-checkpoint <12-hex-id>  # repeatable
```

The default repository scope is derived from the saved session's working
directory basename. If Swift captured
`SWIFT_TAG_HAWK_SESSION_ID=<hawk-session-id>` at its actual session-start
boundary, export automatically resolves the exact Swift session and committed
checkpoints through `swift graph correlation`. Explicit
`--swift-checkpoint` values remain additive. Swift lookup failures never block
Hawk export, and incomplete checkpoint enumeration produces a session link
without unverified checkpoint links.

## Daemon API and SDKs

The daemon exposes the same projection through the authenticated, read-only
endpoint:

```text
GET /v1/sessions/{id}/graph
  ?repository=<scope>
  &swift_checkpoint=<12-hex-id>  # repeatable, maximum 64
```

The endpoint uses the daemon's normal Bearer or `X-API-Key` authentication,
validates session, repository, and checkpoint inputs before projection, and
returns the typed `hawk.graph/v1` envelope. The handler owns transport concerns
only; the Hawk composition root injects the existing graph builder, so CLI,
Cloud sync, and HTTP projections share one construction path.

The Go, Python sync/async, and TypeScript SDK clients expose this endpoint and
validate the returned graph topology before returning it to callers.

After connecting a project with `hawk cloud login` or `hawk cloud connect`, a
completed session snapshot can be uploaded explicitly:

```bash
hawk cloud graph sync [session-id]
  --repository <scope>
  --swift-checkpoint <12-hex-id>  # repeatable
```

The cloud adapter hashes values behind sensitive attribute names, changes those
keys to `*_sha256`, enforces the 250-node/500-edge/500-event/900-total-fact and
1 MiB limits, and derives the sync ID from the prepared graph. Repeating the
same completed snapshot is therefore idempotent. Upload errors are reported to
the explicit command, but cloud availability never affects local execution.
Hawk does not upload prompt bodies, response bodies, tool arguments, tool
results, or other large artifacts.

Mission-scoped graph artifacts can use the same explicit path without being
pretended to be a Hawk session:

```bash
hawk graph export --mission-dir <mission-directory>
hawk cloud graph sync --mission-dir <mission-directory>
```

The CLI reads only `mission-graph.json`, validates its `hawk.graph/v1` schema
and all edge/event references, then applies the same privacy normalization and
Cloud limits. Mission graphs intentionally omit `sessionId`; they are durable
mission facts, not Cloud session telemetry.

## Runtime capture status

The central tool-execution seam automatically records:

- the final permission-engine outcome for every persisted-session tool call;
- enabled human-approval gate outcomes;
- aggregate results from `VerifyPlanExecution`.
- Harrier subgraphs selected by direct, graph-budget, global, proactive, and
  shared-memory retrieval;
- Hawk code-index chunks selected through Harrier-backed code search.
- Merlin scans invoked through the observed bridge/pipeline path.
- Kestrel reviews invoked through the observed bridge path.
- Shrike compression performed by persisted-session context compaction.
- Shrike-only secret matches removed from persisted-session responses.
- Shrike usage summaries and budget decisions emitted from the central,
  deduplicated provider-accounting seam.
- Eyrie model route and usage reported for persisted-session turns.

Mission execution now also persists a portable graph artifact for local mission
runs. Plain `hawk mission` runs emit mission and feature execution nodes; the
graph-driven `hawk mission --from-tasks` path additionally emits operations
nodes for each deterministic wave join, including bounded completion, failure,
and blocked-downstream counts.

Merlin and Kestrel observed bridge calls now emit both their producer-owned
quality topology and Hawk's neutral aggregate verification observation. The
latter contains only failure state, finding count, maximum severity, and a
SHA-256 target digest, allowing `validated_by` composition without retaining
URLs, diffs, findings, evidence, or fixes in the observation journal.

Shrike's tracker now contributes hourly, daily, session, and cost limits to the
pre-turn guard. Existing Hawk cost accounting and limits remain authoritative
and are updated first; Shrike observes the same deduplicated request usage and
provides the additional token-window decision.

### Swift identity handshake contract

Swift now exposes the Swift-owned, read-only
`graph correlation --hawk-session <id>` surface through the permitted
`cli.NewRootCmd()` boundary. It:

- matches only the `SWIFT_TAG_HAWK_SESSION_ID` value stored at Swift's actual
  session-start boundary;
- returns the authoritative Swift session and checkpoint IDs for that Hawk ID;
- never guesses identity from the current branch, HEAD, timestamps, or prompt
  similarity;
- returns an empty match set rather than a speculative match.

Hawk now consumes this surface through `cli.NewRootCmd()`, validates the schema
and echoed Hawk identity, bounds the response, and emits:

- Hawk session `references` Swift session;
- Hawk session `references` each authoritative Swift checkpoint;
- Swift session `produced` Swift checkpoint.

Malformed, mismatched, oversized, unavailable, or incomplete responses fail
open without speculative facts. Explicit `--swift-checkpoint` links remain
available and additive. Permission enforcement and verification gates still
execute before observation emission.
