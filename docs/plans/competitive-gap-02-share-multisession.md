# Gap-02: Share Links + Multi-Session Visibility (Local-First)

Status: Implemented (2026-09-09)
Source: field comparison vs OpenCode share-links/multi-session, herdr multiplexer, Cline checkpoints

Constraint: developer-first, local by default, no cloud account required
(per `.github/ISSUE_TEMPLATE/feature_request.yml:60-63`).

## Existing hawk capabilities (verified)

- `GenerateShareLink` returns local deeplink `hawk://share/<hash[:16]>` (`internal/session/export.go:801-819`); deterministic, no hosted URL.
- Session export (`session_export.go`), mission graph export (`cmd/execution_graph.go`, `mission-graph.json`), daemon sessions, `mission` worktrees.
- Completion list includes `exec`, `daemon`, `mission`, `sessions`, `tools`, `skills` (`cmd/completions_test.go:50`).

## Decision

Adopt: local share bundle + session inventory that works offline.

Do not adopt: hosted share URLs, cloud account,/Desktop app.

## Priority model

- P0: `sessions` list shows id/model/updated + export path; document `graph export` bundle as the share unit.
- P1: TUI session picker surfaces the `hawk://share/<id>` deeplink + export file path for copy-paste.
- P2: Mission watchdog read-only overview (already in HUD panel) exposed via `mission --dry-run`/status; no new runtime.

## Steps

1. Confirm `sessions` command output covers the P0 fields; extend only display, not storage format.
2. Document share flow in `docs/COMPETITIVE.md` + user-guide: export file → send → `graph export` validate.
3. Keep `internal/session` format stable; no contract break.

## Verification

- `go test ./internal/session/ -run TestGenerateShareLink -count=1`.
- `go test ./cmd/ -run TestSession -count=1` (or nearest session-picker test).
- `make vet`.
