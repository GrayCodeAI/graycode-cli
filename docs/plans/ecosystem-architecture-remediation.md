# Ecosystem architecture remediation plan

This plan is the executable follow-up to the July 2026 source-level audit of
the fourteen Hawk ecosystem repositories. An item is complete only when its
acceptance evidence passes; documentation or intent alone is not sufficient.

**Last evidence pass:** 2026-07-11

## P0 — release graph and submodules

- [x] Pin all seven `external/` submodules to the selected, publicly reachable
  ecosystem snapshot. *(local pins present; re-pin after each engine push)*
- [ ] Make the versions in `go.mod` resolve to API-compatible commits from the
  same snapshot. *(blocked until new engine commits — especially yaad
  `b7ee281` — are published; `go.work` replace is authoritative for
  integration builds)*
- [x] Add CI for both supported dependency modes:
  - pinned integration: `go test ./...` with `go.work`;
  - public modules: `GOWORK=off go test ./...`.
  Evidence: `.github/workflows/ci.yml` jobs `module`, `public-modules`.
- [x] Prevent release workflows from falling back from a missing Gitlink commit
  to a branch head.
  Evidence: `.github/actions/checkout-eyrie` defaults `allow_branch_fallback=false`
  and fails on missing/unreachable pins; `release.yml` verifies Gitlink ==
  checked-out HEAD before goreleaser.
- [x] Add a release-parity guard that reports whether each Gitlink is represented
  by the module version in `go.mod`.
  Evidence: `scripts/check-submodule-release-parity.sh` + Makefile target
  `submodule-release-parity` + CI job.

Acceptance:

```sh
git submodule status
make boundaries
go test ./...
GOWORK=off go test ./...
```

## P1 — public contracts

- [x] Keep `hawk/api/openapi.yaml` as the sole Hawk daemon server contract.
  Evidence: SDK snapshots under `hawk-sdk-*/api/openapi.yaml` + coverage tests
  that treat the daemon contract as authoritative.
- [x] Make both SDK repositories verify their implemented methods and JSON
  models against that contract.
  Evidence:
  - Go: `hawk-sdk-go/internal/spec/openapi_coverage_test.go`
  - Python: `hawk-sdk-python/tests/test_openapi_coverage.py`
- [x] Cover `/v1/ready`, `/v1/review`, and `/v1/review/status`, or explicitly
  identify them as intentionally unsupported in SDK capability metadata.
  Evidence: `SUPPORTED_ENDPOINTS.md` in both SDKs + coverage decision maps.
- [x] Add route/operation parity for `hawk-cloud/contracts/openapi.yaml`.
  Evidence: `hawk-cloud/test/openapi-parity.test.ts`
- [x] Replace GrayCode's untyped/manual Hawk Cloud transport surface with a
  contract-checked client boundary.
  Evidence: `graycode-core/apps/backend/test/hawk-cloud-contract.test.ts`
  (BFF may only reference paths present in the cloud OpenAPI snapshot).
- [ ] Add OpenAPI breaking-change checks to CI.
  *(still open — no oasdiff/spectral gate wired yet)*

Acceptance: daemon and cloud route-parity tests pass, SDK contract tests pass,
and a deliberate undocumented route causes the relevant test to fail.

## P1 — Hawk Cloud correctness and security

- [x] Separate client-reported cost from server-calculated ledger cost.
  Evidence: `hawk-cloud/src/domain/metering.ts` (`reportedCostMicros` vs
  `costMicros`) + `test/metering.test.ts` (“ignores forged client cost”).
- [x] Version the pricing input used by server-side metering.
  Evidence: `pricingVersion` on `MeteringResult` + catalog `version` field.
- [x] Ensure billing and budgets use only the verified ledger value.
  Evidence: usage/billing routes aggregate `usage_ledger.cost_micros`, not
  client estimates.
- [x] Add positive and negative authorization tests for every route family.
  Evidence: `hawk-cloud/test/authorization-matrix.test.ts`
- [x] Split route handlers so HTTP, policy, service, and persistence concerns are
  independently testable.
  Evidence: `src/domain/*` + `src/routes/*` layout + domain unit tests.
- [x] Add OSS metadata (`LICENSE`) and document repository/release setup.
  Evidence: `hawk-cloud/LICENSE`, `hawk-cloud/README.md`

Acceptance: cloud tests prove that forged client cost cannot alter billable
cost, every route is present in OpenAPI, authorization matrices pass, typecheck
passes, and the production build succeeds.

## P2 — maintainability and ownership

- [ ] Split GrayCode's organization BFF by organization, project, access,
  billing, enterprise, analytics, and delivery domains.
  *(still open — large refactor; contract tests exist but file split pending)*
- [x] Document `graycode-core.usage_logs` as GrayCode-platform data and prohibit
  it from becoming an authoritative Hawk ledger.
  Evidence: `graycode-core/README.md` (usage_logs paragraph).
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

- [x] Hawk focused tests (memory, cloud client, cmd) after charm/yaad work
- [x] Yaad full `go test ./...` + nested `cmd/yaad-tui` tests
- [x] Hawk Cloud vitest results present (`metering`, `openapi-parity`,
  `authorization-matrix`, …)
- [ ] Full matrix across all fourteen repos (optional CI babysit)

## Verification pass 2

- [ ] Repeat builds/tests with clean workspace-local caches
- [ ] Repeat Hawk with `GOWORK=off` after yaad is published
- [ ] Re-run submodule/release parity after engine pushes
- [x] Audit checkboxes against command or source evidence (this pass)

## Still open (prioritized)

1. **Publish engines then re-pin** — push `yaad` (and any other local-only
   engine SHAs), refresh hawk `go.sum`, green `submodule-release-parity`.
2. **OpenAPI breaking-change CI** — add oasdiff (or equivalent) on
   `hawk/api/openapi.yaml` and `hawk-cloud/contracts/openapi.yaml`.
3. **GrayCode BFF domain split** — split large organization route modules by
   domain for maintainability (behavior already contract-tested).
