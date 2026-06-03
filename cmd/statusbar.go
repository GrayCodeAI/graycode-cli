package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/GrayCodeAI/hawk/internal/engine"
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

	left, leftVis := renderStatusBarLeft()
	right, rightVis := renderStatusBarRight(m)
	return padStatusBarLine(left, right, leftVis, rightVis, width)
}

func renderStatusBarLeft() (rendered string, visLen int) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	display := shortenHomePath(cwd)
	cwdStyle := lipgloss.NewStyle().Foreground(statusCWDColor).Inline(true)
	pathText := display + ":"
	parts := []string{cwdStyle.Render(pathText)}
	visLen = runewidth.StringWidth(pathText)

	if branch, err := gitOutput("rev-parse", "--abbrev-ref", "HEAD"); err == nil && branch != "" {
		if branch == "HEAD" {
			branch, _ = gitOutput("rev-parse", "--short", "HEAD")
		}
		if branch != "" {
			branchStyle := lipgloss.NewStyle().Foreground(statusBranchColor).Inline(true)
			branchText := "⎇ " + branch
			parts = append(parts, branchStyle.Render(branchText))
			visLen += 1 + runewidth.StringWidth(branchText)
		}
	}

	return strings.Join(parts, " "), visLen
}

func renderStatusBarRight(m *chatModel) (rendered string, visLen int) {
	if m == nil || m.session == nil {
		return "", 0
	}

	tokenStyle := lipgloss.NewStyle().Foreground(statusTokenColor).Inline(true)
	costStyle := lipgloss.NewStyle().Foreground(statusCostColor).Inline(true)
	timeStyle := lipgloss.NewStyle().Foreground(hudLabelPink).Inline(true)
	dim := lipgloss.NewStyle().Foreground(dimColor).Inline(true)

	tokens := m.session.Cost.PromptTokens + m.session.Cost.CompletionTokens
	tokenText := "● " + formatTokenCountWithCommas(tokens)
	costText := fmt.Sprintf("$%.2f", m.session.Cost.Total())
	timerText := "⏱ " + formatSessionDuration(sessionDuration(m))

	parts := []string{
		tokenStyle.Render(tokenText),
		costStyle.Render(costText),
		timeStyle.Render(timerText),
	}
	plain := []string{tokenText, costText, timerText}

	if m.vim != nil && m.vim.IsEnabled() {
		vimText := m.vim.ModeString()
		parts = append(parts, dim.Render(vimText))
		plain = append(plain, vimText)
	}

	sep := dim.Render(" · ")
	joined := strings.Join(parts, sep)
	visLen = runewidth.StringWidth(strings.Join(plain, " · "))
	return joined, visLen
}

func sessionDuration(m *chatModel) time.Duration {
	if m == nil || m.startedAt.IsZero() {
		return 0
	}
	return time.Since(m.startedAt)
}

func padStatusBarLine(left, right string, leftVis, rightVis, width int) string {
	gap := width - leftVis - rightVis
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
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

func renderContainerFooterLeft(m chatModel) (rendered string, visLen int) {
	bold, dim := containerFooterLeft(m)
	visLen = runewidth.StringWidth(bold + dim)

	if m.containerEnabled && m.containerErr != nil {
		return containerErrStyle.Bold(true).Render(bold) + containerErrStyle.Render(dim), visLen
	}
	if m.containerEnabled {
		return containerLabelStyle.Render(bold) + renderContainerFooterDetail(dim), visLen
	}

	labelStyle := lipgloss.NewStyle().Foreground(warnAmber).Bold(true)
	return labelStyle.Render(bold) + dimStyle.Render(dim), visLen
}

func renderContainerFooterDetail(detail string) string {
	if detail == "" {
		return ""
	}
	statusStyle := lipgloss.NewStyle().Foreground(textPlaceholder).Inline(true)
	okStyle := lipgloss.NewStyle().Foreground(doneGreen).Inline(true)
	sep := " · "
	status, ok, found := strings.Cut(detail, sep)
	if !found {
		return statusStyle.Render(detail)
	}
	return statusStyle.Render(status) + configMutedStyle().Inline(true).Render(sep) + okStyle.Render(ok)
}

// containerFooterLeft is the bold + dim text on the top footer row (left side).
func containerFooterLeft(m chatModel) (bold, dim string) {
	if !m.containerEnabled {
		return permissionModeLabel(m.session), permissionModeHint(m.session)
	}
	bold = "Container:"
	if m.containerErr != nil {
		return bold, " Docker is not running. Start Docker and try again."
	}
	if m.containerReady && strings.TrimSpace(m.containerStatus) != "" {
		return bold, fmt.Sprintf(" %s · no approval needed", strings.TrimSpace(m.containerStatus))
	}
	if strings.TrimSpace(m.containerStatus) != "" {
		return bold, " " + strings.TrimSpace(m.containerStatus)
	}
	return bold, " starting…"
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

// permissionModeHint returns a short description for the current permission mode.
func permissionModeHint(sess *engine.Session) string {
	if sess == nil || sess.Perm == nil {
		return " - tools require approval"
	}
	switch sess.Perm.Mode {
	case engine.PermissionModeBypassPermissions:
		return " - all tools auto-approved"
	case engine.PermissionModeAcceptEdits:
		return " - file edits auto-approved"
	case engine.PermissionModeDontAsk:
		return " - all tools blocked"
	case engine.PermissionModePlan:
		return " - read-only exploration"
	default:
		return " - tools require approval"
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
	tokens := m.session.Cost.PromptTokens + m.session.Cost.CompletionTokens
	return fmt.Sprintf(
		"Status line (footer)\n  cwd: %s\n  branch: %s\n  gateway: %s\n  model: %s\n  tokens: %s\n  cost: $%.2f\n  duration: %s\n  %s",
		shortenHomePath(cwd),
		strings.TrimSpace(branch),
		gw,
		model,
		formatTokenCountWithCommas(tokens),
		m.session.Cost.Total(),
		formatSessionDuration(sessionDuration(m)),
		m.session.Cost.Summary(),
	)
}
