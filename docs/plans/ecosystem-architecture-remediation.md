# Ecosystem architecture remediation plan

This plan is the executable follow-up to the July 2026 source-level audit of
the fourteen Hawk ecosystem repositories. An item is complete only when its
acceptance evidence passes; documentation or intent alone is not sufficient.

## P0 — release graph and submodules

- [x] Pin all seven `external/` submodules to the selected, publicly reachable
  ecosystem snapshot. *(local pins present; re-pin after each engine push)*
- [ ] Make the versions in `go.mod` resolve to API-compatible commits from the
  same snapshot. *(blocked until new engine commits are published; `go.work`
  replace is authoritative for integration builds)*
- [x] Add CI for both supported dependency modes:
  - pinned integration: `go test ./...` with `go.work`;
  - public modules: `GOWORK=off go test ./...`.
- [ ] Prevent release workflows from falling back from a missing Gitlink commit
  to a branch head. *(verify release.yml still needs hardening)*
- [x] Add a release-parity guard that reports whether each Gitlink is represented
  by the module version in `go.mod`.
  Evidence: `scripts/check-submodule-release-parity.sh` + Makefile target
  `submodule-release-parity`.

Acceptance:

```sh
git submodule status
make boundaries
go test ./...
GOWORK=off go test ./...
```

## P1 — public contracts

- [ ] Keep `hawk/api/openapi.yaml` as the sole Hawk daemon server contract.
- [ ] Make both SDK repositories verify their implemented methods and JSON
  models against that contract.
- [ ] Cover `/v1/ready`, `/v1/review`, and `/v1/review/status`, or explicitly
  identify them as intentionally unsupported in SDK capability metadata.
- [ ] Add route/operation parity for `hawk-cloud/contracts/openapi.yaml`.
- [ ] Replace GrayCode's untyped/manual Hawk Cloud transport surface with a
  contract-checked client boundary.
- [ ] Add OpenAPI breaking-change checks to CI.

Acceptance: daemon and cloud route-parity tests pass, SDK contract tests pass,
and a deliberate undocumented route causes the relevant test to fail.

## P1 — Hawk Cloud correctness and security

- [ ] Separate client-reported cost from server-calculated ledger cost.
- [ ] Version the pricing input used by server-side metering.
- [ ] Ensure billing and budgets use only the verified ledger value.
- [ ] Add positive and negative authorization tests for every route family.
- [ ] Split route handlers so HTTP, policy, service, and persistence concerns are
  independently testable.
- [ ] Add OSS metadata (`LICENSE`) and document repository/release setup.

Acceptance: cloud tests prove that forged client cost cannot alter billable
cost, every route is present in OpenAPI, authorization matrices pass, typecheck
passes, and the production build succeeds.

## P2 — maintainability and ownership

- [ ] Split GrayCode's organization BFF by organization, project, access,
  billing, enterprise, analytics, and delivery domains.
- [ ] Document `graycode-core.usage_logs` as GrayCode-platform data and prohibit
  it from becoming an authoritative Hawk ledger.
- [x] Add import guards for Hawk delivery, application, domain/ports, and adapter
  layers while migration proceeds.
  Evidence: `scripts/check-internal-layer-imports.sh` + `make internal-layers-guard`.
- [x] Define the embedding boundary for Trace and the target boundary between
  Sight source review and Inspect deployed-target inspection.
  Evidence: engine READMEs + `docs/architecture/ecosystem-architecture.md`.
- [x] Decide and document the intentionally narrow `hawk-mcpkit` adoption scope;
  do not force engines with different MCP server requirements into it.
  Evidence: ecosystem architecture table (Sight/Inspect only).
- [x] Reconcile Yaad's implemented, experimental, and planned interface docs.
  Evidence: yaad TUI split to `cmd/yaad-tui` nested module; core library has no
  Bubble Tea deps (2026-07-11).

Acceptance: boundary checks and repository documentation agree with actual
imports and implemented interfaces; all repository test suites pass.

## Verification pass 1

- Run every Go repository's complete test suite.
- Run GrayCode backend/web tests and typechecks.
- Run Hawk Cloud tests, typecheck, format check, and build.
- Run Python SDK and community-skill test suites.
- Run architecture, submodule, route, and contract guards.

## Verification pass 2

- Repeat builds/tests with clean workspace-local caches.
- Repeat Hawk with `GOWORK=off`.
- Re-run submodule/release parity and OpenAPI parity.
- Confirm all fourteen worktrees contain only intentional remediation changes.
- Audit every checkbox above against command or source evidence.
