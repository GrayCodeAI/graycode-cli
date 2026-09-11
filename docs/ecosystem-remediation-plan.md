# Ecosystem Remediation Plan — Post-Audit Findings

This document captures the improvement recommendations from the 2026-07-11
full-ecosystem audit that require significant refactoring and are documented
here for scheduled execution rather than immediate implementation.

## 1. charmbracelet v1/v2 Dependency Duplication (hawk + swift)

**Status:** ✅ Done (2026-07-11)
**Impact:** High — contributed ~20-30MB to hawk binary size

### Outcome

- Hawk and Swift imports migrated to `charm.land/*/v2` (`bubbles`, `bubbletea`,
  `lipgloss`, `huh`, `glamour`).
- Direct `github.com/charmbracelet/{bubbles,bubbletea,lipgloss}` requires removed
  from hawk/swift `go.mod`.
- Binary size gate tightened: **110MB → 80MB** (`make size-check`).
- Verified build: hawk binary **~75 MB** (under gate) after migration.

Commits (hawk): `2890c74` (migrate), `ccfa286` (API fixups), `7a42c1e` (size gate).

### Residual notes

- `github.com/charmbracelet/x/*` and related transitive packages may still appear
  as indirect deps of the v2 stack — that is expected and not dual v1 TUI stack.
- Manual QA checklist (REPL, `/config`, `/autonomy`, swift setup UIs) remains
  recommended when bumping charm majors.

## 2. harrier TUI Dependencies in Library Module

**Status:** ✅ Done (2026-07-11) — Option A
**Impact:** Medium — removed Bubble Tea stack from the core harrier module graph

### Outcome

- Demo TUI moved to nested module `cmd/harrier-tui` with its own `go.mod`
  (`replace github.com/GrayCodeAI/harrier => ../..`).
- Core `github.com/GrayCodeAI/harrier` no longer requires
  `charmbracelet/{bubbles,bubbletea,lipgloss}`.
- Hawk embeds only the library packages (`engine`, `storage`, `graph`, …), so
  the default hawk binary does not pull the demo TUI module.
- Verify: `cd harrier && go test ./...`; `cd harrier/cmd/harrier-tui && go test ./...`.

### Residual notes

- The local `harrier` (Harrier) checkout and Hawk's `go.mod` must resolve to a
  published matching commit. **Publish Harrier before** relying on
  `GOWORK=off` / module-release parity (the Go proxy must see the commit for
  sumdb download).

## 3. shrike Viper vs. Direct TOML Config

**Status:** Documented, requires significant rewrite
**Impact:** Medium — viper pulls 10+ indirect deps (afero, cast, mapstructure, etc.)

### Problem

`shrike/internal/config/config.go` uses `spf13/viper` for:
- Config file search paths
- Environment variable binding (`TOK_*` prefix, aliases)
- `mapstructure` unmarshaling into `Config` struct

But `shrike` already imports `BurntSushi/toml` for direct TOML encode/decode.

### Assessment

Viper is used extensively in `shrike`:
- `v.AutomaticEnv()` + `v.SetEnvPrefix("SHRIKE")` handles env var discovery
- `v.ReadInConfig()` searches multiple config paths
- `v.Unmarshal(cfg)` populates the struct with mapstructure tags
- `bindEnvAliases()` maps 20+ non-standard env var names

Replacing viper with direct `os.Getenv` + `toml.DecodeFile` would require
rewriting the env alias registry and manual config path resolution. This is
~200 lines of non-trivial logic.

### Recommendation

Defer until shrike binary size becomes a priority. The dependency cost is
acceptable for the convenience gained. If addressed, target the `shrike` v1.0
release milestone.

## 4. starling registry.json Build-Time Generation

**Status:** Documented, requires CI pipeline change
**Impact:** Medium — 4.3MB JSON committed to git, grows unbounded

### Problem

`registry.json` is a 4.3MB generated file committed to git. It is updated by
`python tools/update_registry.py` after skill additions/removals. This creates
large diffs and bloats the repo.

### Remediation Steps

1. **Remove `registry.json` from git:** `git rm registry.json` and add to
   `.gitignore`.
2. **Generate at build/publish time:** Run `python tools/update_registry.py`
   in CI before publishing artifacts.
3. **Runtime fetch:** Have `hawk` fetch the registry from a CDN or GitHub
   raw URL rather than embedding it.

### Risk

If `registry.json` is read offline by hawk, removing it requires adding a
fetch-or-cache mechanism. Verify how hawk consumes the registry before
removing from git.

---

*Audit date: 2026-07-11*
*Auditor: Droid (Factory AI)*
