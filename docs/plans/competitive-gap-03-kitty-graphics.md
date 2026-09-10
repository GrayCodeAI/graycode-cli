# Gap-03: Kitty Graphics Protocol for Terminal Images

Status: Implemented (2026-09-09)
Source: `https://github.com/kovidgoyal/kitty` (GPL-3.0; protocol only, no code copy), Ghostty compat; extends `docs/plans/pi-adoption-plan.md:25` (already proposed there — this file scopes the TUI work, it does not re-propose).

## Existing graycode capabilities (verified)

- Vision input path: `internal/engine/vision.go`; image command: `cmd/image.go`.
- Terminal detection covers kitty/ghostty/wezterm/alacritty names (`internal/ui/icons/detect_test.go:56`).
- No Kitty graphics emit path found in source audit 2026-09-08.

## Decision

Adopt: Kitty graphics emit for image display with capability detection + text fallback.

Do not adopt: kitty source, GPL code, Ghostty/Zig code, breaking Bubble Tea v2 rendering.

## Priority model

- P0: capability probe (env `KITTY_PID`/terminfo/`TERM_PROGRAM` + query) with safe fallback to current rendering.
- P1: wire probe into image/screenshot display path (`cmd/image.go`, vision output).
- P2: chunked transmit + resize policy for large PNGs.

## Steps

1. Add `internal/tui/graphics.go` (new, isolated): probe + encode + emit + fallback. No imports from kitty.
2. Gate behind explicit detection; default behavior unchanged on non-capable terminals.
3. Tests: unit probe/encode tests with golden byte prefixes; no live terminal required.

## Verification

- `go test ./internal/tui/ -run TestGraphics -count=1` (new).
- `go test ./cmd/ -run TestImage -count=1` (existing path unbroken).
- `make vet`.
