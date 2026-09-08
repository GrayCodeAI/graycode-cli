# Gap-05: Default Wiring for Media / Computer-Use / STT Backends

Status: Proposed
Source: field comparison vs Qwen `computer_use`, Codex browser/screenshot, Goose extensions; README notes router ships `ImageClient`/`AudioClient`

Constraints (non-negotiable):

- Provider ownership lives in `../graycode-router`; graycode consumes only the stable engine facade (`AGENTS.md`, `docs/SECURITY-DEVELOPER.md:51-56`).
- Boundary guards must stay green: `make boundaries` (incl. `graycode-router-client-guard`, `graycode-router-engine-guard`).
- Tools fail safe today with explicit errors when unwired — preserve that behavior when disabled.

## Existing graycode capabilities (verified)

- `ComputerUseTool` reports "no computer backend installed" without `SetComputerBackend` (`internal/tool/computer_use.go:67-94`).
- `GenerateMediaTool` backend nil by default via `SetMediaEngine` (`internal/tool/media_generation.go:69-71`).
- `internal/stt` package exists (`stt.go`); Telegram voice path documented as backend-seamed.
- Custom providers supported via settings (`internal/config/settings.go:50,115`).

## Decision

Adopt: opt-in host wiring through the router facade, env-gated, off by default.

Do not adopt: direct `graycode-router/client` production imports, new secrets paths, always-on media/computer-use.

## Priority model

- P0: design note mapping each tool seam → facade method (media/image, STT/audio, computer backend) with guard-safe import path.
- P1: env-gated wiring (e.g. `GRAYCODE_MEDIA=1`) + docs; unwired default error text unchanged.
- P2: `graycode doctor` reports backend status (wired/unwired) without leaking secrets.

## Steps

1. Confirm facade methods exist in sibling `../graycode-router/engine`; if missing, file the change there first (router repo owns providers).
2. Implement wiring in graycode behind env gates; keep `Set*` seams for tests.
3. Run `make boundaries` + `make vet` + targeted `go test ./internal/tool/ -run 'TestComputer|TestMedia'`.

## Verification

- `make boundaries` green (no client-boundary violation).
- `go test ./internal/tool/ -run 'TestComputerUse|TestMediaGeneration' -count=1`.
- `make vet`.
