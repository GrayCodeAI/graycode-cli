package cmd

// theme.go — the single source of truth for hawk's visual identity.
//
// All color constants (24-bit RGB via lipgloss), raw ANSI escape codes
// (used by the spinner line), and glyph/icon constants live here. Every
// other file in the package references these names instead of repeating
// hex codes or magic strings. To rebrand hawk, edit this file; to audit
// what's used where, grep this file.
//
// Organization:
//   1. Brand & identity
//   2. UI state (active, selected, focus)
//   3. Semantic feedback (success, warning, error, info)
//   4. Tooling & agents
//   5. HUD overlay
//   6. Status bar metrics
//   7. Text hierarchy
//   8. Structure (borders, backgrounds, separators)
//   9. Spinner-line ANSI escapes (raw, not lipgloss)
//  10. Icons & glyphs

import "github.com/charmbracelet/lipgloss"

// ---------------------------------------------------------------------------
// 1. Brand & identity
// ---------------------------------------------------------------------------

// hawkColor is the brand orange (#FF5E0E). Used for the HAWK wordmark,
// mascot, ⛬ assistant prefix, prompt arrow, cursor, exit prompt, and
// any place that should "speak" as the brand.
var hawkColor = lipgloss.Color("#FF5E0E")

// ---------------------------------------------------------------------------
// 2. UI state
// ---------------------------------------------------------------------------

// The selected/focused item uses brand orange. Active/current values use
// configActiveStyle so they remain visually distinct from the cursor.

// ---------------------------------------------------------------------------
// 3. Semantic feedback
// ---------------------------------------------------------------------------

// successTeal — success, ready, free, ✓ indicators.
var successTeal = lipgloss.Color("#4ECDC4")

// warnAmber — warning, permission title, caution.
var warnAmber = lipgloss.Color("#FFB347")

// errorCoral — error (anywhere — generic, status, container).
var errorCoral = lipgloss.Color("#FF6B6B")

// infoSky — info, status CWD, container-adjacent (though see
// containerBlue for Docker-specific labels).
var infoSky = lipgloss.Color("#75B1E2")

// ---------------------------------------------------------------------------
// 4. Tooling & agents
// ---------------------------------------------------------------------------

// toolGold — tool name, action identifier.
var toolGold = lipgloss.Color("#F1C40F")

// agentGold — agent title. Slightly lighter than toolGold so the two
// read as related but distinct (agent = the person, tool = the thing).
var agentGold = lipgloss.Color("#FFD93D")

// doneGreen — agent "done"/success state.
var doneGreen = lipgloss.Color("#4CAF50")

// containerBlue — Docker container label. Distinct from infoSky so
// container output reads as its own zone in the status footer.
var containerBlue = lipgloss.Color("#3BAADA")

// Autonomy tier colors (Scout → Builder → Operator → Autonomous), coolest to hottest.
var (
	tierInspect = lipgloss.Color("#75B1E2") // infoSky — read-only
	tierEdit    = lipgloss.Color("#4ECDC4") // successTeal — default work
	tierRun     = lipgloss.Color("#FFB347") // warnAmber — shell
	tierTrust   = lipgloss.Color("#FF6B6B") // errorCoral — minimal gates
)

// ---------------------------------------------------------------------------
// 5. HUD overlay
// ---------------------------------------------------------------------------

// hudBorderPurple — HUD panel border. Distinct from the brand so the
// panel reads as an overlay zone, not part of the main TUI.
var hudBorderPurple = lipgloss.Color("#9B59B6")

// hudLabelPink — HUD field label. Distinct from the HUD border so
// label and container don't merge visually.
var hudLabelPink = lipgloss.Color("#FF69B4")

// ---------------------------------------------------------------------------
// 6. Status bar metrics
// ---------------------------------------------------------------------------

// costViolet — session cost in the status bar.
var costViolet = lipgloss.Color("#C678DD")

// branchYellow — git branch in the status bar.
var branchYellow = lipgloss.Color("#E5C07B")

// tokenSage — session token count in the status bar.
var tokenSage = lipgloss.Color("#98C379")

// cwdBlue — current working directory in the status bar. Same as
// infoSky by design — CWD is informational.
var cwdBlue = lipgloss.Color("#75B1E2")

// ---------------------------------------------------------------------------
// 7. Text hierarchy (five levels of visual weight for prose)
// ---------------------------------------------------------------------------

// textPrimary — body text, primary prose.
var textPrimary = lipgloss.Color("#F0F0F0")

// textMuted — secondary/muted prose.
var textMuted = lipgloss.Color("#9E9E9E")

// textPlaceholder — input field placeholder.
var textPlaceholder = lipgloss.Color("#7A7A7A")

// textDisabled — disabled, idle, slash menu (non-selected), blockquote,
// markdown HR.
var textDisabled = lipgloss.Color("#666666")

// textWhite — bright text (permission body, code foreground).
var textWhite = lipgloss.Color("#FFFFFF")

// ---------------------------------------------------------------------------
// 8. Structure (borders, backgrounds, separators)
// ---------------------------------------------------------------------------

// borderDim — input/panel/divider border.
var borderDim = lipgloss.Color("#555555")

// bgCode — code block background.
var bgCode = lipgloss.Color("#2A2A3A")

// ---------------------------------------------------------------------------
// 9. Spinner-line ANSI escapes (raw, not lipgloss)
//
// The spinner line is built with raw ANSI escape strings (not lipgloss
// styles) for two reasons: (a) it's hot-path code that runs every render
// frame, and (b) we want explicit control over which attributes combine
// (e.g. cyan + italic). Each constant here is one foreground color
// rendered at the same bright intensity so the line reads as a uniform
// strip.
//
//   glyph/verb/dot: spinnerWaveColors — 20-color flowing wave (spinner_wave.go)
//   time:   ansiBlue     — elapsed seconds
//   ↓:      ansiMagenta  — live model output
//   ↑:      ansiCyan     — session context input
//   sep:    ansiWhite    — structural divider
//   dim:    ansiDim      — secondary/disabled (empty dots, hints)
// ---------------------------------------------------------------------------

const (
	ansiOrange  = "\033[38;2;255;94;14m"
	ansiGreen   = "\033[92m"
	ansiYellow  = "\033[93m"
	ansiBlue    = "\033[94m"
	ansiMagenta = "\033[95m"
	ansiCyan    = "\033[96m"
	ansiWhite   = "\033[97m"
	ansiTeal    = "\033[38;2;78;205;196m"  // matches successTeal — spinner elapsed
	ansiCoral   = "\033[38;2;255;107;107m" // matches errorCoral
	ansiAmber   = "\033[38;2;255;179;71m"  // matches warnAmber
	ansiGrayDim = "\033[38;2;102;102;102m" // matches textDisabled
	ansiDim     = "\033[2m"
	ansiItalic  = "\033[3m"
	ansiBold    = "\033[1m"
	ansiReset   = "\033[0m"
)

// ---------------------------------------------------------------------------
// 10. Icons & glyphs
//
// Hawk's icon registry lives in internal/ui/icons. Every call site
// references icons.ChevronRight() / icons.Robot() / etc. directly; this
// file no longer holds any glyph constants. The audit test in
// internal/testaudit fails CI if any non-ASCII literal appears in
// cmd/*.go outside of the icons package.
//
// The short-lived package-level vars below exist ONLY so existing call
// sites that read `iconPrompt`/`iconAssistantPrefix`/etc. continue to
// compile during the migration. They mirror icons.*() values at the
// time the package is initialized. Prefer icons.X() directly in new
// code; these will be deleted in a follow-up.
// ---------------------------------------------------------------------------

// quitAgainMsg is a static user-facing string and lives here because it
// is not a glyph. Kept in this file to avoid moving a one-line constant.
const quitAgainMsg = "Press Ctrl+C again to quit."
