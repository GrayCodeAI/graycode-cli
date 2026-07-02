package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
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
	statusTokenColor  = tokenSage
	statusCostColor   = costViolet
)

// renderStatusBar renders the session stats footer below the input area.
// Left: cwd + git branch. Right: tokens · cost · session duration.
func renderStatusBar(m *chatModel, width int) string {
	if width < 20 {
		width = 80
	}
	left := renderStatusBarLeft(m)
	right := renderStatusBarRight(m)
	return layoutFooterRow(left, right, width)
}

// statusBranchTTL bounds how long the cwd+branch segment is cached, so a
// branch switch shows up in the status bar within a few seconds.
const statusBranchTTL = 5 * time.Second

func renderStatusBarLeft(m *chatModel) string {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	if m != nil && m.statusLeftKey == cwd && m.statusLeftVal != "" &&
		time.Since(m.statusLeftAt) < statusBranchTTL {
		return m.statusLeftVal
	}
	display := shortenHomePath(cwd)
	cwdStyle := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true)
	pathText := display + ":"
	parts := []string{cwdStyle.Render(pathText)}

	if branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD"); err == nil && branch != "" {
		if branch == "HEAD" {
			branch, _ = gitOutput("rev-parse", "--short", "HEAD")
		}
		if branch != "" {
			branchStyle := lipgloss.NewStyle().Foreground(statusBranchColor).Inline(true)
			branchText := icons.Branch() + " " + branch
			parts = append(parts, branchStyle.Render(branchText))
		}
	}

	out := strings.Join(parts, " ")
	if m != nil {
		m.statusLeftKey = cwd
		m.statusLeftVal = out
		m.statusLeftAt = time.Now()
	}
	return out
}

func renderStatusBarRight(m *chatModel) string {
	if m == nil || m.session == nil {
		return ""
	}

	tokenStyle := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true)
	costStyle := lipgloss.NewStyle().Foreground(statusCostColor).Inline(true)
	timeStyle := lipgloss.NewStyle().Foreground(hudLabelPink).Inline(true)
	focusStyle := lipgloss.NewStyle().Foreground(infoSky).Inline(true)
	dim := lipgloss.NewStyle().Foreground(dimColor).Inline(true)

	tokens := m.session.CostValue().PromptTokens + m.session.CostValue().CompletionTokens
	tokenText := icons.Database() + " " + formatTokenCountCompact(tokens) + " tokens"
	costText := fmt.Sprintf("%s %.2f", icons.Ruby(), m.session.CostValue().Total())
	var meta []string
	if m.inScrollbackFocus() {
		meta = append(meta, focusStyle.Render("⧉"))
	}
	if m.waiting && !m.streamFollow {
		meta = append(meta, dim.Render(icons.Pause()))
	}

	parts := append(
		meta,
		tokenStyle.Render(tokenText),
		costStyle.Render(costText),
	)

	sessionDur := time.Duration(0)
	if !m.sessionStartedAt.IsZero() {
		sessionDur = time.Since(m.sessionStartedAt)
	}
	clockText := icons.ClockOutline() + " " + formatSessionDuration(sessionDur)
	parts = append(parts, timeStyle.Render(clockText))

	if m.waiting || m.manualCompacting {
		timerText := formatSessionDuration(requestDuration(m))
		parts = append(parts, timeStyle.Render(timerText))
	}

	if m.vim != nil && m.vim.IsEnabled() {
		parts = append(parts, dim.Render(m.vim.ModeString()))
	}
	return strings.Join(parts, dim.Render(" · "))
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

	if m.containerEnabled && m.containerErr != nil {
		return containerErrStyle.Bold(true).Render(bold) + containerErrStyle.Render(dim)
	}
	if m.containerEnabled {
		return containerLabelStyle.Render(bold) + renderContainerFooterDetail(dim, m.session)
	}

	labelStyle := lipgloss.NewStyle().Foreground(warnAmber).Bold(true)
	return labelStyle.Render(bold) + dimStyle.Render(dim)
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
	if !m.containerEnabled {
		return "Host mode:", hostModeHint(m.session)
	}
	bold = "Container:"
	if m.containerErr != nil {
		return bold, " Docker is not running. Start Docker and try again."
	}
	if m.containerReady && strings.TrimSpace(m.containerStatus) != "" {
		tier := "Builder"
		if m.session != nil && m.session.PermSvc().Autonomy() != 0 {
			tier = autonomyTierName(m.session.PermSvc().Autonomy())
		}
		status := shortenFooterContainerStatus(strings.TrimSpace(m.containerStatus))
		return bold, fmt.Sprintf(" %s · %s", status, tier)
	}
	if strings.TrimSpace(m.containerStatus) != "" {
		return bold, " " + strings.TrimSpace(m.containerStatus)
	}
	return bold, " starting…"
}

func hostModeHint(sess *engine.Session) string {
	if sess == nil || sess.Perm == nil {
		return " commands run on your machine · ask before tools"
	}
	switch sess.Perm.Mode {
	case engine.PermissionModeBypassPermissions:
		return " commands run on your machine · tools auto-approved"
	case engine.PermissionModeAcceptEdits:
		return " commands run on your machine · auto-approve edits"
	case engine.PermissionModeDontAsk:
		return " commands run on your machine · tools blocked"
	case engine.PermissionModePlan:
		return " commands run on your machine · read-only exploration"
	default:
		return " commands run on your machine · ask before tools"
	}
}

// permissionModeLabel returns the display label for the current permission mode.
func permissionModeLabel(sess *engine.Session) string {
	if sess == nil || sess.Perm == nil {
		return "Default"
	}
	switch sess.Perm.Mode {
	case engine.PermissionModeBypassPermissions:
		return "Bypass (All Allowed)"
	case engine.PermissionModeAcceptEdits:
		return "Auto (Edits Allowed)"
	case engine.PermissionModeDontAsk:
		return "Deny (All Blocked)"
	case engine.PermissionModePlan:
		return "Plan (Read Only)"
	default:
		return "Default"
	}
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
