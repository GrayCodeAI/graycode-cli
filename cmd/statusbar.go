package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

var (
	// Status bar — each metric its own hue so the bar reads as a
	// strip of distinct information. Aliases for the global palette
	// (in case callers want to use these names directly).
	statusCWDColor    = cwdBlue
	statusBranchColor = branchYellow
	statusSpecColor   = infoSky
	statusTokenColor  = tokenSage
	statusCostColor   = costViolet

	statusCwdStyle          = lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true)
	statusBranchStyle       = lipgloss.NewStyle().Foreground(statusBranchColor).Inline(true)
	statusSpecStyle         = lipgloss.NewStyle().Foreground(statusSpecColor).Inline(true)
	statusTokenStyle        = lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true)
	statusCostStyle         = lipgloss.NewStyle().Foreground(statusCostColor).Inline(true)
	statusClockStyle        = lipgloss.NewStyle().Foreground(hudLabelPink).Inline(true)
	statusFocusStyle        = lipgloss.NewStyle().Foreground(infoSky).Inline(true)
	statusDimStyle          = lipgloss.NewStyle().Foreground(dimColor).Inline(true)
	dryRunStyle             = lipgloss.NewStyle().Foreground(warnAmber).Bold(true).Inline(true)
	containerModeStyle      = lipgloss.NewStyle().Foreground(successTeal).Bold(true).Inline(true)
	containerModeErrStyle   = lipgloss.NewStyle().Foreground(errorCoral).Bold(true).Inline(true)
	containerModeMutedStyle = lipgloss.NewStyle().Foreground(dimColor).Bold(true).Inline(true)
)

// renderStatusBar renders the session stats footer below the input area.
// Returns 1 line normally, or 2 lines when the terminal is wide enough
// (width >= 120) and secondary operational state is present.
func renderStatusBar(m *chatModel, width int) []string {
	if width < 20 {
		width = 80
	}
	left := renderStatusBarLeft(m)
	right := renderStatusBarRight(m)
	if width >= 120 {
		// Two-line layout: primary row keeps cwd/branch/model + tokens/cost.
		// Secondary row gets autonomy tier, container mode, session ID, hints.
		primary := layoutFooterRow(renderStatusBarPrimaryLeft(m), renderStatusBarPrimaryRight(m), width)
		secondary := layoutFooterRow(renderStatusBarSecondaryLeft(m), renderStatusBarSecondaryRight(m), width)
		if secondary == "" || secondary == strings.Repeat(" ", len(secondary)) {
			return []string{primary}
		}
		return []string{primary, secondary}
	}
	return []string{layoutFooterRow(left, right, width)}
}

// renderStatusBarPrimaryLeft — cwd, branch, spec stage.
func renderStatusBarPrimaryLeft(m *chatModel) string {
	cwd, ok := cachedStatusLeftCwd(m)
	if !ok {
		return ""
	}
	parts := []string{statusCwdStyle.Render(cwd + ":")}
	if branch := cachedStatusBranch(m); branch != "" {
		parts = append(parts, statusBranchStyle.Render(icons.Branch()+" "+branch))
	}
	if stage := specStageForStatus(m); stage != "" {
		parts = append(parts, statusSpecStyle.Render(stage))
	}
	return strings.Join(parts, statusDimStyle.Render(" "))
}

// renderStatusBarPrimaryRight — tokens, cost, duration.
func renderStatusBarPrimaryRight(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}
	tokens := m.session.CostValue().PromptTokens + m.session.CostValue().CompletionTokens
	tokenText := icons.Database() + " " + formatTokenCountCompact(tokens) + " tokens"
	costText := fmt.Sprintf("%s %.2f", icons.Ruby(), m.session.CostValue().Total())
	parts := []string{
		statusTokenStyle.Render(tokenText),
		statusCostStyle.Render(costText),
	}
	sessionDur := time.Duration(0)
	if !m.sessionStartedAt.IsZero() {
		sessionDur = time.Since(m.sessionStartedAt)
	}
	parts = append(parts, statusClockStyle.Render(icons.ClockOutline()+" "+formatSessionDuration(sessionDur)))
	if m.waiting || m.manualCompacting {
		parts = append(parts, statusClockStyle.Render(formatSessionDuration(requestDuration(m))))
	}
	return strings.Join(parts, statusDimStyle.Render(" · "))
}

func renderStatusBarSecondaryLeft(m *chatModel) string {
	return ""
}

// renderStatusBarSecondaryRight — errors, dry-run, vim, focus/pause.
func renderStatusBarSecondaryRight(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}
	var parts []string
	if m.inScrollbackFocus() {
		parts = append(parts, statusFocusStyle.Render("⧉"))
	}
	if m.waiting && !m.streamFollow {
		parts = append(parts, statusDimStyle.Render(icons.Pause()))
	}
	// Container mode is already shown in the top footer row (containerFooterLeft),
	// so we only surface it here when there's an error to report.
	if m.containerEnabled && m.containerErr != nil {
		parts = append(parts, containerModeErrStyle.Render("▣ container error"))
	}
	if m.session != nil && m.session.PermSvc() != nil && m.session.PermSvc().DryRun() {
		parts = append(parts, dryRunStyle.Render(icons.Pause()+" DRY-RUN"))
	}
	if m.vim != nil && m.vim.IsEnabled() {
		parts = append(parts, statusDimStyle.Render(m.vim.ModeString()))
	}
	return strings.Join(parts, statusDimStyle.Render(" · "))
}

// statusBranchTTL bounds how long the cwd+branch segment is cached, so a
// branch switch shows up in the status bar within a few seconds.
const statusBranchTTL = 5 * time.Second

func (m *chatModel) refreshStatusBarLeft(force bool) bool {
	if m == nil {
		return false
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	if !force && m.statusLeftKey == cwd && m.statusLeftVal != "" && time.Since(m.statusLeftAt) < statusBranchTTL {
		return false
	}
	branch := ""
	if b, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD"); err == nil && b != "" {
		branch = b
		if branch == "HEAD" {
			branch, _ = gitOutput("rev-parse", "--short", "HEAD")
		}
	}
	m.statusLeftKey = cwd
	m.statusLeftVal = shortenHomePath(cwd)
	m.statusLeftBranch = branch
	m.statusLeftAt = time.Now()
	return true
}

func renderStatusBarLeft(m *chatModel) string {
	cwd, ok := cachedStatusLeftCwd(m)
	if !ok {
		return ""
	}
	parts := []string{statusCwdStyle.Render(cwd + ":")}
	if branch := cachedStatusBranch(m); branch != "" {
		parts = append(parts, statusBranchStyle.Render(icons.Branch()+" "+branch))
	}
	if stage := specStageForStatus(m); stage != "" {
		parts = append(parts, statusSpecStyle.Render(stage))
	}
	return strings.Join(parts, statusDimStyle.Render(" "))
}

// specStageForStatus returns a short spec stage indicator for the status bar,
// or empty string if no spec workflow is active.
func specStageForStatus(m *chatModel) string {
	if m == nil || m.session == nil || m.session.Perm == nil {
		return ""
	}
	stage := m.session.Perm.Stage
	if stage == engine.SpecStageNone {
		return ""
	}
	label := specStageDisplayName(stage)
	if stage == engine.SpecStageImplementing && m.session.Perm.Phases > 0 {
		return fmt.Sprintf("%s %s %d/%d", icons.FileDocument(), label, m.session.Perm.Phase, m.session.Perm.Phases)
	}
	return fmt.Sprintf("%s %s", icons.FileDocument(), label)
}

func cachedStatusLeftCwd(m *chatModel) (string, bool) {
	if m == nil {
		cwd, err := os.Getwd()
		if err != nil {
			return ".", false
		}
		return shortenHomePath(cwd), true
	}
	if m.statusLeftVal == "" {
		return "", false
	}
	return m.statusLeftVal, true
}

func cachedStatusBranch(m *chatModel) string {
	if m == nil {
		branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
		if err != nil || branch == "" {
			return ""
		}
		if branch == "HEAD" {
			branch, _ = gitOutput("rev-parse", "--short", "HEAD")
		}
		return branch
	}
	return m.statusLeftBranch
}

func renderStatusBarRight(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}

	tokens := m.session.CostValue().PromptTokens + m.session.CostValue().CompletionTokens
	tokenText := icons.Database() + " " + formatTokenCountCompact(tokens) + " tokens"
	costText := fmt.Sprintf("%s %.2f", icons.Ruby(), m.session.CostValue().Total())
	var meta []string
	if m.inScrollbackFocus() {
		meta = append(meta, statusFocusStyle.Render("⧉"))
	}
	if m.waiting && !m.streamFollow {
		meta = append(meta, statusDimStyle.Render(icons.Pause()))
	}

	parts := append(
		meta,
		statusTokenStyle.Render(tokenText),
		statusCostStyle.Render(costText),
	)

	// Context window usage — compact bar showing how full the context is.
	if m.session != nil {
		if used := sessionContextUsedTokens(m.session); used > 0 {
			if window := m.session.ContextWindowSize(); window > 0 {
				pct := int(float64(used) / float64(window) * 100)
				if pct > 999 {
					pct = 999
				}
				ctxText := fmt.Sprintf("ctx %d%%", pct)
				parts = append(parts, statusDimStyle.Render(ctxText))
			}
		}
	}

	sessionDur := time.Duration(0)
	if !m.sessionStartedAt.IsZero() {
		sessionDur = time.Since(m.sessionStartedAt)
	}
	clockText := icons.ClockOutline() + " " + formatSessionDuration(sessionDur)
	parts = append(parts, statusClockStyle.Render(clockText))

	if m.waiting || m.manualCompacting {
		timerText := formatSessionDuration(requestDuration(m))
		parts = append(parts, statusClockStyle.Render(timerText))
	}

	// Prominent dry-run indicator — safety-critical awareness.
	if m.session != nil && m.session.PermSvc() != nil && m.session.PermSvc().DryRun() {
		parts = append(parts, dryRunStyle.Render(icons.Pause()+" DRY-RUN"))
	}

	if m.vim != nil && m.vim.IsEnabled() {
		parts = append(parts, statusDimStyle.Render(m.vim.ModeString()))
	}
	// Persistent autonomy tier indicator — always visible for safety awareness.
	if m.session != nil && m.session.PermSvc() != nil {
		level := effectivePermissionTier(m.session)
		parts = append(parts, autonomyTierStyle(level).Render("◈ "+autonomyTierName(level)))
	}

	// Persistent Docker isolation indicator — safety-critical awareness.
	if m.containerReady {
		parts = append(parts, containerModeStyle.Render(icons.Shield()+" isolated"))
	} else if m.containerErr != nil || !m.containerEnabled {
		parts = append(parts, containerModeErrStyle.Render(icons.Alert()+" Docker required"))
	} else {
		parts = append(parts, containerModeMutedStyle.Render(icons.Container()+" starting"))
	}

	return strings.Join(parts, statusDimStyle.Render(" · "))
}

func sessionDuration(m *chatModel) time.Duration {
	if m == nil {
		return 0
	}
	start := m.sessionStartedAt
	if start.IsZero() {
		start = m.startedAt
	}
	if start.IsZero() {
		return 0
	}
	return time.Since(start)
}

func requestDuration(m *chatModel) time.Duration {
	if m == nil || m.startedAt.IsZero() {
		return 0
	}
	return time.Since(m.startedAt)
}

func shortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func formatSessionDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSecs := int(d.Seconds())
	if totalSecs < 60 {
		return fmt.Sprintf("%ds", totalSecs)
	}
	mins := totalSecs / 60
	secs := totalSecs % 60
	if mins >= 60 {
		return fmt.Sprintf("%dh %dm", mins/60, mins%60)
	}
	return fmt.Sprintf("%dm %ds", mins, secs)
}

func formatTokenCountWithCommas(tokens int) string {
	p := message.NewPrinter(language.English)
	return p.Sprintf("%d tokens", tokens)
}

func renderContainerFooterLeft(m chatModel) string {
	bold, dim := containerFooterLeft(m)

	if m.containerErr != nil || !m.containerEnabled {
		return containerErrStyle.Bold(true).Render(bold) + containerErrStyle.Render(dim)
	}
	return containerLabelStyle.Render(bold) + renderContainerFooterDetail(dim, m.session)
}

func renderContainerFooterDetail(detail string, sess *engine.Session) string {
	if detail == "" {
		return ""
	}
	statusStyle := lipgloss.NewStyle().Foreground(textPlaceholder).Inline(true)
	sep := " · "
	status, tierPart, found := strings.Cut(detail, sep)
	if !found {
		return statusStyle.Render(detail)
	}
	var level engine.AutonomyLevel
	if sess != nil && sess.PermSvc().Autonomy() != 0 {
		level = sess.PermSvc().Autonomy()
	} else {
		level = autonomyLevelForTierName(tierPart)
	}
	return statusStyle.Render(status) + configMutedStyle().Inline(true).Render(sep) + autonomyTierStyle(level).Render(strings.TrimSpace(tierPart))
}

// containerFooterLeft is the bold + dim text on the top footer row (left side).
func containerFooterLeft(m chatModel) (bold, dim string) {
	bold = icons.Container() + " Docker:"
	if !m.containerEnabled {
		return bold, " required · agent tools locked"
	}
	if m.containerErr != nil {
		return bold, " unavailable · press r to retry"
	}
	if m.containerReady && strings.TrimSpace(m.containerStatus) != "" {
		tier := "Builder"
		if m.session != nil && m.session.PermSvc().Autonomy() != 0 {
			tier = autonomyTierName(m.session.PermSvc().Autonomy())
		}
		status := shortenFooterContainerStatus(strings.TrimSpace(m.containerStatus))
		if stage := currentSpecStage(m.session); stage != engine.SpecStageNone && stage != engine.SpecStageImplementing {
			return bold, fmt.Sprintf(" %s · %s · spec:%s", status, tier, specStageDisplayName(stage))
		}
		return bold, fmt.Sprintf(" %s · %s", status, tier)
	}
	if strings.TrimSpace(m.containerStatus) != "" {
		status := strings.TrimSpace(m.containerStatus)
		if !m.containerReady && m.containerErr == nil {
			// Show activity indicator during active boot so the user sees progress.
			return bold, fmt.Sprintf(" ◐ %s", status)
		}
		return bold, " " + status
	}
	if !m.containerReady && m.containerErr == nil {
		return bold, " ◐ starting…"
	}
	return bold, " starting…"
}

func statusLineSummary(m *chatModel) string {
	if m == nil || m.session == nil {
		return "no active session"
	}
	cwd, _ := os.Getwd()
	branch, _ := gitOutput("rev-parse", "--abbrev-ref", "HEAD")
	if branch == "HEAD" {
		branch, _ = gitOutput("rev-parse", "--short", "HEAD")
	}
	gw, model, _ := m.connectionStatusParts()
	tokens := m.session.CostValue().PromptTokens + m.session.CostValue().CompletionTokens
	return fmt.Sprintf(
		"Status line (footer)\n  cwd: %s\n  branch: %s\n  gateway: %s\n  model: %s\n  tokens: %s\n  cost: $%.2f\n  duration: %s\n  %s",
		shortenHomePath(cwd),
		strings.TrimSpace(branch),
		gw,
		model,
		formatTokenCountWithCommas(tokens),
		m.session.CostValue().Total(),
		formatSessionDuration(sessionDuration(m)),
		m.session.CostValue().Summary(),
	)
}
