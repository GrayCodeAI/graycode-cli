# Ecosystem Verification Status — 2026-07-13

## Verdict

The audited revision set has a release-aligned Hawk-face/Eyrie-engine boundary.
Local gates, Eyrie's release gates, and Hawk's final hosted pull-request gates
are green. The remaining publication step is the signed Hawk release tag and
its generated artifacts.

The architecture and focused hardening tests support this responsibility split:

```text
users and SDKs
      |
      v
Hawk product face
  CLI / daemon / agent loop / sessions / tools / permissions / product schemas
      |
      v
Eyrie engine facade
  credentials / provider state / catalog / route resolution / transport /
  normalized streams / provider resilience / provider telemetry
      |
      v
model providers
```

The prior Eyrie release-parity mismatch is resolved by v0.2.1. Hawk PR #92 and
its follow-up documentation sync in PR #93 passed their hosted checks and were
merged to `main`.

## Verified responsibility boundary

Hawk owns the user-facing product and orchestration concerns:

- CLI, TUI, daemon and SDK entrypoints
- coding-agent loop, tool authorization, permissions and project policy
- product session history, WAL, checkpoints, resume and replay
- task-semantic model intent and user-visible configuration presentation
- Hawk-owned persistence, runtime and public response schemas

Eyrie owns the provider engine concerns behind `eyrie/engine`:

- credentials, provider state and safe status projection
- catalog discovery, model capabilities and concrete route resolution
- provider adapters, transport and normalized generation/streaming
- provider retry, timeout, fallback, health and telemetry behavior

Hawk production code is guarded against lower-level Eyrie imports. Hawk may
record product-level latency and usage, but it must not reimplement provider
routing or apply a second resilience policy to an Eyrie-facade request.

## Hardening present in the audited workspace

### Resolved route attribution

- Hawk's provider-neutral response and stream DTOs preserve Eyrie's resolved
  route.
- `route_selected` and `route_changed` events update the effective provider and
  model used by traces, hooks, usage events and cost accounting.
- Cost updates change the effective model and apply token/cost totals under one
  lock, preventing a routed fallback from being billed as the requested model.
- A repeated terminal usage payload is de-duplicated without dropping distinct
  continuation-segment usage.

Focused blocking, streaming, partial-route, usage and cost tests passed,
including the focused race checks recorded during this audit.

### One provider-resilience layer

- Hawk's `ChatClient` compatibility port has an optional resilience-ownership
  capability. The Eyrie facade adapter advertises that it owns provider
  resilience; legacy injected clients do not.
- For an Eyrie facade stream, Hawk delegates the initial request exactly once
  and bypasses Hawk's compatibility call retry/rate limiting, transient stream
  reopen, thinking-only non-streaming fallback and synthetic `max_tokens`
  continuation.
- Legacy clients retain those compatibility behaviors. This preserves tests and
  third-party adapters without wrapping production Eyrie calls in a second
  retry policy.
- Hawk remains responsible for authorizing tool calls and persisting product
  conversation messages regardless of which client owns resilience.

Focused engine tests, focused race tests, `go vet` and the Hawk/Eyrie boundary
guards passed for this split.

### Lossless Eyrie selection and provider-state migration

- Existing Eyrie active selection is authoritative over Hawk's legacy
  `provider` and `model` settings.
- A legacy provider/model pair is validated and written as one selection before
  its source fields are removed. Rejected selections leave the source settings
  untouched for repair instead of silently discarding them.
- Historical provider-state secrets are imported into Eyrie's secret store
  before an atomic sanitized rewrite.
- Eyrie accepts the historical decode-only `version` key as well as canonical
  `_version`, while still rejecting unknown fields, trailing JSON, conflicting
  versions and unsupported future versions.

Standalone Eyrie passed the full Go test suite, the full race-enabled suite,
`go vet`, both ecosystem boundary guards and `git diff --check` in this audit.

### Hawk daemon readiness and durable sessions

- `GET /v1/ready` returns success only when a session factory exists and
  Eyrie's local preflight reports `Ready=true`. A missing or failed probe
  returns 503 with a reason; factory wiring alone is not provider readiness.
- `POST /v1/chat` without a session ID creates a random durable ID, persists the
  transcript and returns the same ID in the JSON response and
  `X-Hawk-Session-ID` header.
- A request with a session ID requires an existing durable session, inherits
  its transcript and metadata, appends the new turn and persists under the same
  ID. Invalid IDs return 400 and missing sessions return 404.
- SSE responses expose the session ID header, persist the conversation and put
  the session ID and usage in the final `done` event.
- Corrupt or unreadable session state is reported as an internal persistence
  failure instead of being misclassified as a missing session. Fixed lock
  striping serializes same-session operations without letting arbitrary
  client-supplied IDs grow a lifetime lock map.
- Hawk applies and persists the requested agent persona. Session CWD is
  validated and canonicalized as durable metadata; daemon tools intentionally
  continue to use the daemon's startup CWD rather than an unsafe process-wide
  directory change.

Focused daemon/session and command tests passed in isolated directories, and
the daemon package passed its race-enabled test run. This preflight is a local
configuration/readiness check, not proof of live remote-provider authentication.

### Hawk Cloud usage queue

- The idempotency marker insert and monthly rollup update execute in one D1
  batch. The rollup uses SQLite `changes()` from the marker insert, so a
  duplicate delivery cannot increment the aggregate twice.
- A failed message is retried rather than acknowledged at an application-local
  retry limit. Cloudflare Queue `max_retries` and dead-letter configuration are
  the single terminal-delivery policy, avoiding silent event loss.
- Retry backoff is capped at 12 hours.

The queue-focused tests, the then-current full Hawk Cloud test suite,
type-check, formatting check, Wrangler type generation/dry run and a direct
SQLite idempotency check passed during the audit.

### CI gates

- Hawk's Docker Trivy step now fails on fixable high or critical image findings
  (`exit-code: '1'`).
- Hawk Cloud CI now generates a V8 JSON coverage report, fails if the report or
  any metric is missing, and enforces all four metric floors.
- The honest initial coverage floors are 30% statements, 20% branches, 40%
  functions and 35% lines. The final measured local baseline was 32.65%,
  23.84%, 40.64% and 36.37%, respectively. Sixty percent remains a ratchet
  target, not a description of current coverage.

These are locally verified workflow/configuration changes. A successful remote
CI run on the final published commits is still required.

### Community-skill corpus hardening

- All 12,167 discovered skills pass the full-corpus validator.
- Every warning category is at zero: broken internal references, path traversal,
  oversized files and `SKILL.md` bodies, script shebang and executable-bit
  defects, excess tags, overlong descriptions and uncategorized warnings.
- The checked-in warning budget is zero per category. CI requires an exact
  match, new categories start at zero and the budget must never increase.
- The local-reference validator covers every Markdown file in each skill,
  applies exact-case and skill-root containment checks, and ignores code spans,
  fenced code, anchors and external URLs.
- Safe, dry-run-first cleanup and oversized-body migration tools preserve
  readable content and frontmatter while moving large bodies into ordered
  progressive-disclosure references. The size allowlist now has zero
  exceptions.

The current community repository suite passed 303 tests, and the full validator
reported 12,167 passed, zero failed and zero warnings. Ruff, boundary and
registry checks remain part of the repository gate; this local result does not
replace final remote CI.

## Verification evidence captured

| Scope | Evidence recorded during this audit | Status |
| --- | --- | --- |
| Standalone Eyrie | full tests, full race tests, vet, boundary guards, diff check | Passed |
| Hawk route/config seams | focused tests and focused race checks | Passed |
| Hawk daemon/session seams | focused daemon, session and command tests; daemon race test | Passed |
| Support Go repos | full tests and vet for `hawk-core-contracts`, `inspect`, `sight`, `tok`, `yaad`, `trace`, `hawk-mcpkit` and `hawk-sdk-go` | Passed |
| Python SDK | 288 tests, Ruff check/format and strict mypy | Passed |
| Hawk Cloud queue | focused and full tests, type-check, format, Wrangler checks, direct SQLite check | Passed |
| Hawk full integration | isolated full tests, full race tests, vet and all architecture guards against the completed workspace | Passed |
| Published release graph | Eyrie v0.2.1 gitlink/module parity; two full Hawk passes in workspace and `GOWORK=off` modes | Passed locally and in Hawk hosted CI |
| Community skills | 303 tests; 12,167 skills passed; zero failures and zero warnings; Ruff, boundary and registry gates | Passed locally with a zero-warning budget |
| Adjacent GrayCode Core | forced 266-test run, lint, type-check, production build, Hawk Cloud contract comparison and package audit | Passed; not a runtime dependency |

Passing a row describes the recorded local evidence only. It does not replace a
clean checkout, public revision reachability, signed release or remote CI run.

The final security sweep found no reachable Go vulnerabilities in Hawk or
Eyrie, no npm audit findings in Hawk Cloud or GrayCode Core, and no known Python
SDK dependency vulnerabilities. Hawk's verbose Go scan did note
`GO-2026-5932` at module level because `golang.org/x/crypto` contains the
unmaintained `openpgp` package; no Hawk import or call reaches that package and
the advisory has no fixed module version. GrayCode Core remains outside the
Hawk runtime graph; its only checked integration here is the versioned Hawk
Cloud API contract.

## Release status

### Eyrie release graph is aligned

The release sequence completed without weakening the parity guard or copying
local Eyrie source into Hawk:

| Source | Revision |
| --- | --- |
| Published Eyrie module | `v0.2.1` |
| Published tag and module origin | `2e5ec4e3bb03705d5a09792009f113625258fc5a` |
| Hawk `external/eyrie` checkout and gitlink candidate | `2e5ec4e3bb03705d5a09792009f113625258fc5a` |

The Eyrie v0.2.1 GitHub Release is published. Its pull-request gates passed
tests with the race detector, coverage, lint, vet, module hygiene, security,
four fuzz targets and all configured cross-platform builds. The module checksum
is recorded in Hawk's `go.sum`, and `go mod download -json` resolves v0.2.1 to
the same commit as the clean submodule checkout.

Hawk then passed two complete shuffled test runs through the workspace checkout
and two through `GOWORK=off`, including the public-module build. The exact
workspace race-and-coverage run passed at 69.0% total statement coverage. Vet,
lint, formatting, all architecture guards, module verification and both
workspace/module vulnerability scans passed.

### Hawk hosted CI passed

The completed architecture change set passed Hawk's hosted test, race,
coverage, boundary, security, public-module, compatibility-matrix, Docker, and
submodule-parity gates on the exact reviewable revision before merge. Release
publication still independently verifies the tagged revision and artifacts.

## Production-readiness exit criteria

The ecosystem can make a production-ready claim only after all of the following
are true in one reproducible final revision set:

- Eyrie Gitlink, clean checkout and published Go module resolve to the same
  public commit.
- Hawk passes full, race, vet, boundary and release-parity verification through
  both the local submodule and `GOWORK=off` module graph.
- Hawk Cloud tests, coverage gate, type-check, deployment dry run and security
  scans pass in remote CI.
- Community-skill validation remains at zero warnings in every category, with
  no size exceptions, in the final clean revision.
- The final repository set is clean, reviewable and tagged; no required behavior
  depends on uncommitted local patches.
