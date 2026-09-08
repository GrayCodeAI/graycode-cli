# Gap-01: Docker-Onboarding Friction (Docs/UX Only)

Status: Proposed
Source: field comparison vs Codex (net-off workspace-write), Gemini (gVisor/sandbox-exec profiles), Pi (no sandbox)

Constraint (non-negotiable): mandatory Docker isolation, fail-closed, never host fallback.
See `docs/SECURITY-DEVELOPER.md:71-73` and `internal/sandbox/container.go:72-74`.
This plan adds zero execution paths. It only improves messaging/docs.

## Existing graycode capabilities (verified)

- `graycode path` / `preflight` / `doctor` / `ecosystem` commands (`README.md:346-356`).
- `scripts/verify-developer-path.sh` (`make path`) and `scripts/smoke-graycode.sh` (`make smoke`).
- Sandbox image auto-pull (`graycodeai/graycode-sandbox`) with local Dockerfile build fallback (`docs/SECURITY-DEVELOPER.md:75-79`).

## Decision

Adopt: clearer failure copy + ordered remediation when Docker is missing.

Do not adopt: workspace/host execution tier, `--yolo`-style host bypass, silent fallback.

## Priority model

- P0: `path`/`preflight`/`doctor` emit the same ordered checklist (daemon running? image cached? registry reachable? local build available?).
- P1: README quick-start callout that Docker is required before first run (already stated; tighten wording + link to checklist).
- P2: `smoke` output pastes the failing step with the exact fix command.

## Steps

1. Audit current `path`, `preflight`, `doctor` outputs for divergent Docker messages.
2. Unify copy: state fail-closed explicitly, then ordered steps (start daemon → pull → local build).
3. Update `README.md` install section link to `docs/SECURITY-DEVELOPER.md` checklist.
4. No changes to `internal/sandbox`, `internal/engine`, permissions.

## Verification

- `make path` with Docker stopped prints ordered checklist (manual).
- `go test ./cmd/ -run 'TestPath|TestPreflight|TestDoctor' -count=1`.
- `make vet`.
