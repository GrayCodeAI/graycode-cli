# How Go CLIs handle icons — a survey

This is the survey done on 2026-06-16 when picking the icon strategy for
hawk. The question was: how do other Go CLI/TUI projects render icons in
a terminal, and should we use Lucide (the project's visual identity for
docs) or a different approach?

## Methodology

- Cloned or fetched main-branch Go source from each project via GitHub.
- Counted occurrences of `U+1F300–U+1FAFF` (emoji block) and
  `U+2600–U+27BF` (dingbat block) in the source.
- Examined the rendering primitives they use (spinner, icon helper, etc.)
- Cross-referenced with their docs and READMEs.

## Findings

| Project | Emoji in source? | Approach |
|---|---|---|
| [charmbracelet/glow](https://github.com/charmbracelet/glow) | 0 | Pure box-drawing + braille in markdown rendering. |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) (spinner) | 0 (braille U+28xx only) | Spinner uses `"⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷"` — braille patterns, not emoji. |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | 0 | Color and style only. |
| [charmbracelet/mods](https://github.com/charmbracelet/mods) | 0 | ASCII only. |
| [charmbracelet/soft-serve](https://github.com/charmbracelet/soft-serve) | 0 | ASCII only. |
| [charmbracelet/pop](https://github.com/charmbracelet/pop) | 0 | ASCII only. |
| [spf13/cobra](https://github.com/spf13/cobra) | 0 | ASCII only. |
| [github/cli](https://github.com/cli/cli) (`pkg/iostreams/color.go`) | 1 (`✓`) | `ColorScheme.SuccessIcon()` returns the literal `"✓"`. Warning and failure icons are ASCII (`"!"`, `"X"`). |
| [derailed/k9s](https://github.com/derailed/k9s) | 3 (`🐶`, `💣`, `✅`) | Hard-coded emoji in startup slog messages. No central icon helper. |
| [epilande/go-devicons](https://github.com/epilande/go-devicons) | 0 (Nerd Font PUA only) | The closest "icon library" for Go: maps file paths to Nerd Font PUA codepoints. No ASCII fallback. |
| **hawk (this project)** | **0** (audit-enforced) | Centralized `internal/ui/icons` registry. Nerd Font PUA codepoints in Nerd Font mode, ASCII tokens in ASCII mode. Terminal-capability detection (TTY, NO_COLOR, LANG) gates the mode. |

## Why not Lucide?

Lucide (<https://lucide.dev>) is the project's visual identity for docs
and web surfaces (see `docs/architecture.md`, which embeds Lucide SVGs).
It is an SVG-only icon set — **there is no standard PUA mapping for
Lucide in Nerd Fonts**. The Nerd Fonts cheat sheet
(<https://www.nerdfonts.com/cheat-sheet>) confirms the available icon
sets are: `nf-cod-*` (VS Code Codicons), `nf-fa-*` (FontAwesome),
`nf-mdi-*` (Material Design), `nf-oct-*` (GitHub Octicons), `nf-pom-*`
(Pomicons), `nf-seti-*` (Seti-UI), `nf-pl-*` (Powerline), and
language / weather extras. None of these are Lucide.

The only ways to render Lucide glyphs in a terminal are:

1. **Build a custom Nerd Font** that embeds a Lucide subset and ship
   it. Requires font-forge / Python tooling, and a way to ensure the
   end user has the patched font installed. None of the popular Go
   CLIs surveyed do this.
2. **Render Lucide SVGs as Unicode block art at print time.** Use
   half-block characters (`▀ ▄ ▌ ▐`) to compose a 2-color image
   from an SVG path. Real Lucide look in any terminal, but ~10× slower
   printing, complex code, and breaks for captured output (the
   "▀▄" sequence doesn't diff well). The popular
   [`jp2a`](https://github.com/cslarsen/jp2a) tool does this for
   JPEGs, but for inline CLI icons it's a non-starter.
3. **Use a different PUA-based icon set that resembles Lucide.** The
   current state — Nerd Fonts Codicons — is the closest practical
   match. Codicons share a 2px stroke, rounded geometry with Lucide,
   and look familiar to anyone who's used a recent IDE. They are not
   Lucide, but they don't try to be.

## What hawk does

The `internal/ui/icons` package implements option (3) with the
following design:

- Every glyph in the registry has a Nerd Font PUA codepoint and an
  ASCII fallback token.
- A `Mode()` function returns `ModeNerd` or `ModeASCII` based on:
  - The `HAWK_ICONS=nerd|ascii` env var (forces a mode).
  - `NO_COLOR` set → `ModeASCII` (also disables ANSI color).
  - stdout not a TTY → `ModeASCII` (captured output stays clean).
  - A Nerd Font detected in the terminal → `ModeNerd`.
  - Locale looks like UTF-8 → `ModeNerd`; otherwise → `ModeASCII`.
- The `TestNoEmojiInCmd` and `TestNoEmojiInInternalExceptIcons` audits
  parse every non-test Go file in `cmd/` and `internal/`, fail CI on
  any emoji (U+1F300–U+1FAFF) or dingbat (U+2600–U+27BF) rune. Parser
  files (`test_loop.go`, `test_fixtures.go`) and markdown / multiagent
  prompt packages are exempt with a documented comment.

The end result: hawk's CLI is portable, fast, mode-aware, and
guaranteed emoji-free by an enforced audit. Users with a Nerd Font
get icons; everyone else gets readable ASCII tokens.

## Verdict

The Nerd-Font-PUA + ASCII-fallback approach is the de facto standard
for Go CLIs that care about icon rendering. The hawk approach is more
disciplined than most (centralized registry, auto-detected mode,
audit-enforced emoji ban, mode-aware tests). Migrating to Lucide is
not feasible in a terminal without one of the expensive options
above; the current path is the right call.
