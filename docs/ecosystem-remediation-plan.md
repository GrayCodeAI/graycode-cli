# Ecosystem Remediation Plan — Post-Audit Findings

This document captures the improvement recommendations from the 2026-07-11
full-ecosystem audit that require significant refactoring and are documented
here for scheduled execution rather than immediate implementation.

## 1. charmbracelet v1/v2 Dependency Duplication (hawk + trace)

**Status:** Documented, requires migration sprint
**Impact:** High — contributes ~20-30MB to hawk binary size

### Problem

`hawk` and `trace` both import `charmbracelet` v1 packages alongside
`charm.land` v2 packages:

| v1 Package | v2 Package | Used In |
|------------|------------|---------|
| `charmbracelet/bubbles` | `charm.land/bubbles/v2` | hawk, trace |
| `charmbracelet/bubbletea` | `charm.land/bubbletea/v2` | hawk, trace |
| `charmbracelet/lipgloss` | `charm.land/lipgloss/v2` | hawk, trace |
| `charmbracelet/x/ansi` | `charm.land/x/ansi` | hawk, trace |

`go.mod` analysis shows `hawk` imports both versions simultaneously. This
is likely a half-finished migration.

### Remediation Steps

1. **Audit import usage:** Run `go list -m -json all | jq -r '.Path' | grep charm`
   in both `hawk` and `trace` to identify every v1 import.
2. **Migrate hawk TUI code:** Update all `charmbracelet/bubbletea` imports to
   `charm.land/bubbletea/v2` in `hawk/internal/...` TUI packages.
3. **Migrate trace TUI code:** Update all `charmbracelet/*` imports to
   `charm.land/*/v2` in `trace/cli/...` and `trace/internal/...`.
4. **Remove v1 from go.mod:** After all imports are migrated, run
   `go mod tidy` to drop the v1 modules.
5. **Verify binary size:** Run `make size-check` in hawk — expect ~20-30MB
   reduction.

### Risk

Breaking TUI behavior. The v2 APIs have subtle differences in event handling,
styling, and input processing. Requires full manual QA of:
- hawk interactive REPL
- hawk `/config` picker
- hawk `/autonomy` tier picker
- trace `enable` interactive setup
- trace `checkpoint rewind` selection UI

## 2. yaad TUI Dependencies in Library Module

**Status:** Documented, requires optional-build-tag refactor
**Impact:** Medium — bloats hawk binary with unnecessary TUI deps

### Problem

`yaad` is a library embedded in `hawk`, but its `go.mod` imports:
- `charmbracelet/bubbles`
- `charmbracelet/bubbletea`
- `charmbracelet/lipgloss`

These are only needed for a demo/TUI interface, not for the core memory engine.

### Remediation Options

**Option A (preferred):** Move TUI code to `cmd/yaad-demo/` with its own `go.mod`.
- Create `cmd/yaad-demo/go.mod` with `replace` to parent module
- Move TUI packages from `yaad/internal/` to `cmd/yaad-demo/`
- Add `//go:build yaad_tui` tag to remaining TUI code in library
- hawk imports yaad without the tag → no TUI deps

**Option B:** Add `//go:build yaad_tui` to all TUI source files.
- Less invasive than Option A
- hawk never builds with `yaad_tui` tag
- Still requires audit of which files are TUI-only

### Verification

After refactor, `go list -m -deps github.com/GrayCodeAI/yaad | wc -l` should
show fewer indirect dependencies. The hawk binary should shrink by ~5-10MB.

## 3. tok Viper vs. Direct TOML Config

**Status:** Documented, requires significant rewrite
**Impact:** Medium — viper pulls 10+ indirect deps (afero, cast, mapstructure, etc.)

### Problem

`tok/internal/config/config.go` uses `spf13/viper` for:
- Config file search paths
- Environment variable binding (`TOK_*` prefix, aliases)
- `mapstructure` unmarshaling into `Config` struct

But `tok` already imports `BurntSushi/toml` for direct TOML encode/decode.

### Assessment

Viper is used extensively in `tok`:
- `v.AutomaticEnv()` + `v.SetEnvPrefix("TOK")` handles env var discovery
- `v.ReadInConfig()` searches multiple config paths
- `v.Unmarshal(cfg)` populates the struct with mapstructure tags
- `bindEnvAliases()` maps 20+ non-standard env var names

Replacing viper with direct `os.Getenv` + `toml.DecodeFile` would require
rewriting the env alias registry and manual config path resolution. This is
~200 lines of non-trivial logic.

### Recommendation

Defer until tok binary size becomes a priority. The dependency cost is
acceptable for the convenience gained. If addressed, target the `tok` v1.0
release milestone.

## 4. hawk-community-skills registry.json Build-Time Generation

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
