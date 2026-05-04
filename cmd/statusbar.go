package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/GrayCodeAI/hawk/engine"
)

// renderStatusBar renders a full-width status bar for the chat TUI.
// Left: model name (dimmed)
// Center: cost ($0.00) | tokens (0k) | messages (0)
// Right: session duration (if >1min) | vim mode indicator (if active)
func renderStatusBar(m *chatModel, width int) string {
	if width < 20 {
		width = 80
	}

	dimSty := lipgloss.NewStyle().Foreground(dimColor)
	tealSty := lipgloss.NewStyle().Foreground(tealColor)

	// Left: model name
	modelName := m.session.Model()
	if modelName == "" {
		modelName = "no model"
	}
	left := dimSty.Render(modelName)
	leftVisLen := runewidth.StringWidth(modelName)

	// Center: cost | tokens | messages
	cost := m.session.Cost.Total()
	tokenEst := m.session.Cost.PromptTokens + m.session.Cost.CompletionTokens
	msgCount := m.session.MessageCount()

	costStr := fmt.Sprintf("$%.2f", cost)
	tokenStr := formatTokenCount(tokenEst)
	msgStr := fmt.Sprintf("%d msgs", msgCount)

	centerText := fmt.Sprintf("%s | %s | %s", costStr, tokenStr, msgStr)
	center := tealSty.Render(centerText)
	centerVisLen := runewidth.StringWidth(centerText)

	// Right: session duration (if >1min) | vim mode (if active)
	var rightParts []string
	if !m.startedAt.IsZero() {
		dur := time.Since(m.startedAt)
		if dur > time.Minute {
			minutes := int(dur.Minutes())
			if minutes >= 60 {
				rightParts = append(rightParts, fmt.Sprintf("%dh%dm", minutes/60, minutes%60))
			} else {
				rightParts = append(rightParts, fmt.Sprintf("%dm", minutes))
			}
		}
	}
	if m.vim != nil && m.vim.IsEnabled() {
		rightParts = append(rightParts, m.vim.ModeString())
	}

	rightText := strings.Join(rightParts, " | ")
	right := dimSty.Render(rightText)
	rightVisLen := runewidth.StringWidth(rightText)

	// Calculate spacing
	totalUsed := leftVisLen + centerVisLen + rightVisLen
	remaining := width - totalUsed
	if remaining < 2 {
		// Compressed: just show left and center
		gap := width - leftVisLen - centerVisLen
		if gap < 1 {
			gap = 1
		}
		return left + strings.Repeat(" ", gap) + center
	}

	leftGap := remaining / 2
	rightGap := remaining - leftGap

	return left + strings.Repeat(" ", leftGap) + center + strings.Repeat(" ", rightGap) + right
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

// formatTokenCount formats a token count for display.
func formatTokenCount(tokens int) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM tok", float64(tokens)/1_000_000)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%.1fK tok", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d tok", tokens)
}
