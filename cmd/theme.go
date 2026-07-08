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

import (
	"github.com/charmbracelet/lipgloss"

	internaltheme "github.com/GrayCodeAI/hawk/internal/theme"
)

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

// Neutral text/structure colors are adaptive: the Dark variant is the
// original value (so dark terminals — the default — render identically),
// and the Light variant is dark ink so prose and borders stay legible on
// light terminals, where the old near-white values were invisible.
// lipgloss selects the variant from the detected terminal background.

// textPrimary — body text, primary prose.
var textPrimary = lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#F0F0F0"}

// textMuted — secondary/muted prose.
var textMuted = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#9E9E9E"}

// textPlaceholder — input field placeholder.
var textPlaceholder = lipgloss.Color("#7A7A7A")

// textDisabled — disabled, idle, slash menu (non-selected), blockquote,
// markdown HR.
var textDisabled = lipgloss.AdaptiveColor{Light: "#A0A0A0", Dark: "#666666"}

// textWhite — bright text (permission body, code foreground).
var textWhite = lipgloss.Color("#FFFFFF")

// ---------------------------------------------------------------------------
// 8. Structure (borders, backgrounds, separators)
// ---------------------------------------------------------------------------

// borderDim — input/panel/divider border.
var borderDim = lipgloss.AdaptiveColor{Light: "#C6C6C6", Dark: "#555555"}

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
	ansiOrange   = "\033[38;2;255;94;14m"
	ansiGreen    = "\033[92m"
	ansiYellow   = "\033[93m"
	ansiBlue     = "\033[94m"
	ansiMagenta  = "\033[95m"
	ansiCyan     = "\033[96m"
	ansiWhite    = "\033[97m"
	ansiTeal     = "\033[38;2;78;205;196m"  // matches successTeal — spinner elapsed
	ansiCoral    = "\033[38;2;255;107;107m" // matches errorCoral
	ansiAmber    = "\033[38;2;255;179;71m"  // matches warnAmber
	ansiGrayDim  = "\033[38;2;102;102;102m" // matches textDisabled
	ansiDone     = "\033[38;2;76;175;80m"   // matches doneGreen — diff additions
	ansiSky      = "\033[38;2;117;177;226m" // matches infoSky — diff hunk headers
	ansiContBlue = "\033[38;2;59;170;218m"  // matches containerBlue — diff file headers
	ansiDim      = "\033[2m"
	ansiItalic   = "\033[3m"
	ansiBold     = "\033[1m"
	ansiReset    = "\033[0m"
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

// ---------------------------------------------------------------------------
// 11. Live theme switching
// ---------------------------------------------------------------------------

// ApplyTheme mutates all global color vars to match the named palette so
// that every lipgloss.NewStyle() call issued after this returns correctly
// themed styles. Call this at startup (from root.go) and whenever the user
// selects a new theme from the picker.
//
// "auto" is a no-op: lipgloss already probes the terminal background on
// its own; the caller should still call lipgloss.SetHasDarkBackground for
// the AdaptiveColor vars but the fixed-hex vars remain at their defaults.
func ApplyTheme(name string) {
	if name == "" || name == "auto" {
		return
	}
	entry, ok := internaltheme.LookupTheme(name)
	if !ok {
		return
	}
	p := entry.Palette

	// 1. Brand — accent color from the palette.
	hawkColor = lipgloss.Color(p.Accent)

	// 2. Semantic feedback.
	successTeal = lipgloss.Color(p.Green)
	warnAmber = lipgloss.Color(p.Amber)
	errorCoral = lipgloss.Color(p.Red)
	infoSky = lipgloss.Color(p.Blue)

	// 3. Tooling & agents — reuse semantic colors from palette.
	toolGold = lipgloss.Color(p.Amber)
	agentGold = lipgloss.Color(p.Accent)
	doneGreen = lipgloss.Color(p.Green)
	containerBlue = lipgloss.Color(p.Blue)

	// 4. Autonomy tier colors.
	tierInspect = lipgloss.Color(p.Blue)
	tierEdit = lipgloss.Color(p.Green)
	tierRun = lipgloss.Color(p.Amber)
	tierTrust = lipgloss.Color(p.Red)

	// 5. HUD overlay — use accent.
	hudBorderPurple = lipgloss.Color(p.Accent)
	hudLabelPink = lipgloss.Color(p.Accent)

	// 6. Status bar.
	costViolet = lipgloss.Color(p.Accent)
	branchYellow = lipgloss.Color(p.Amber)
	tokenSage = lipgloss.Color(p.Green)
	cwdBlue = lipgloss.Color(p.Blue)

	// 7. Text hierarchy.
	textPrimary = lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: p.Ink}
	textMuted = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: p.Muted}
	textPlaceholder = lipgloss.Color(p.Faint)
	textDisabled = lipgloss.AdaptiveColor{Light: "#A0A0A0", Dark: p.Faintest}

	// 8. Structure.
	borderDim = lipgloss.AdaptiveColor{Light: "#C6C6C6", Dark: p.Line2}
	bgCode = lipgloss.Color(p.Panel)

	// 9. Update dark-background flag so AdaptiveColors resolve correctly.
	lipgloss.SetHasDarkBackground(entry.IsDark)

	// 10. Rebuild package-level styles that snapshotted the old colors at
	// init time. Without this, markdown/chat/agent-grid styles keep the
	// default palette after a live theme switch.
	refreshThemeStyles()
}

// refreshThemeStyles rebuilds every package-level lipgloss.Style var that
// captures a theme color at package-init time. ApplyTheme mutates the color
// vars, but styles constructed from them hold copies — so each one must be
// reconstructed after a palette swap.
//
// Like ApplyTheme itself, this must only run on the Bubble Tea Update
// goroutine (or before the program starts); the style vars are read
// unsynchronized from View.
func refreshThemeStyles() {
	// markdown.go
	mdHeaderStyle = lipgloss.NewStyle().Foreground(successTeal).Bold(true)
	mdBoldStyle = lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
	mdInlineCodeStyle = lipgloss.NewStyle().Background(bgCode).Foreground(textPrimary)
	mdCodeBlockStyle = lipgloss.NewStyle().Background(bgCode)
	mdCodeLabelStyle = lipgloss.NewStyle().Foreground(textDisabled).Background(bgCode)
	mdLinkTextStyle = lipgloss.NewStyle().Foreground(successTeal)
	mdLinkURLStyle = lipgloss.NewStyle().Foreground(textDisabled)
	mdBlockquoteBar = lipgloss.NewStyle().Foreground(textDisabled)
	mdBlockquoteText = lipgloss.NewStyle().Foreground(textDisabled)
	mdHRStyle = lipgloss.NewStyle().Foreground(textDisabled)
	mdBulletStyle = lipgloss.NewStyle().Foreground(successTeal)

	// chat_model.go
	dimStyle = lipgloss.NewStyle().Foreground(textDisabled)
	errorStyle = lipgloss.NewStyle().Foreground(errorCoral)
	warnStyle = lipgloss.NewStyle().Foreground(warnAmber)
	toolStyle = lipgloss.NewStyle().Foreground(toolGold).Bold(true)
	toolDimStyle = lipgloss.NewStyle().Foreground(textDisabled)
	slashCmdStyle = lipgloss.NewStyle().Foreground(textDisabled)
	slashDescStyle = lipgloss.NewStyle().Foreground(textDisabled)
	slashSelCmdStyle = lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
	slashSelDescStyle = lipgloss.NewStyle().Foreground(hawkColor)
	inputBorderStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true, false, true, false).BorderForeground(borderDim)
	containerErrStyle = lipgloss.NewStyle().Foreground(errorCoral)
	containerLabelStyle = lipgloss.NewStyle().Foreground(containerBlue)
	dimColor = textDisabled

	// agent_grid.go
	agentActiveStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(hawkColor).Padding(0, 1)
	agentDoneStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(doneGreen).Padding(0, 1)
	agentFailStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(errorCoral).Padding(0, 1)
	agentIdleStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(textDisabled).Padding(0, 1)
	agentTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(agentGold)
	agentStatusStyle = lipgloss.NewStyle().Foreground(textMuted)

	// chat_scrollbar.go
	scrollbarThumbStyle = lipgloss.NewStyle().Foreground(hawkColor)
}
