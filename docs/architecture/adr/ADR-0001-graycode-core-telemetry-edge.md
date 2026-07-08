# ADR-0001: Constrained telemetry edge from hawk to graycode-core

- Status: Accepted
- Date: 2026-07-05
- Owners: hawk maintainers

## Context

The architecture docs declare `hawk runtime -> graycode-core` a forbidden
edge: `graycode-core` is a company/platform repo (billing, accounts, usage
dashboards), not a Hawk runtime dependency, and Hawk must remain fully
functional as an OSS tool without it.

At the same time, `graycode-core`'s backend already anticipates
hawk-ecosystem data: its `POST /usage/log` and activity routes validate a
`tool` enum of `hawk | trace | tok | yaad | inspect | sight`. Without a
written rule, the first person to wire usage reporting into hawk will either
violate the forbidden edge or invent an ad-hoc mechanism with unclear
boundaries.

Today no hawk code calls graycode-core. This ADR exists to constrain that
edge *before* it appears.

## Decision

The compile-time rule is unchanged and absolute:

- No repo in the hawk ecosystem (hawk, engines, contracts, SDKs, mcpkit,
  skills) may import graycode-core code, its packages, or its generated
  clients. `engine -> graycode-core` stays forbidden with no exception.

A single, narrow *runtime* exception is sanctioned:

- `hawk` (the product repo, and only hawk) MAY send usage/activity telemetry
  to graycode-core's public HTTP API, subject to all of the following:
  1. **Opt-in.** Disabled by default. Enabled only by explicit user
     configuration (e.g. logging into a GrayCodeAI account). Never enabled
     by an install script or a default config file.
  2. **Fail-open.** Every failure mode — endpoint unreachable, auth expired,
     schema rejected — degrades to "no telemetry". It must never block,
     delay, or alter an agent session, and must never surface as a user
     -facing error in normal operation.
  3. **HTTP-only, contract by API.** The integration speaks graycode-core's
     published HTTP API. No shared Go/TS types, no vendored client, no
     graycode-core dependency in `go.mod`.
  4. **Owned by hawk.** Engines never report telemetry themselves; hawk
     aggregates and reports on their behalf. The `tool` field in the payload
     may name an engine, but the caller is always hawk.
  5. **Isolated.** The integration lives in a single hawk package (e.g.
     `internal/platform/`), behind an interface, so removing it is a
     one-package deletion.

## Consequences

- The forbidden-edges lists in `hawk-current-vs-proposed.md` and
  `hawk-ecosystem-summary.md` now read
  `hawk runtime -> graycode-core (compile time; runtime telemetry only per ADR-0001)`.
- Boundary-check tooling keeps rejecting any `graycode-core` import in any
  hawk-ecosystem repo; nothing in this ADR weakens that check.
- If graycode-core's API changes incompatibly, hawk telemetry silently stops
  until hawk updates — acceptable by the fail-open rule.
- Any broadening of this edge (engines reporting directly, hawk *reading*
  platform state at runtime, shared type packages) requires a new ADR.
