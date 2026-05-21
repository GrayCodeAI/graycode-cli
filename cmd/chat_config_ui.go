package cmd

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func configMutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
}

func configTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
}

func configSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
}

func configRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))
}

func configHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6")).Bold(true)
}

func configNoticeStyle(notice string) lipgloss.Style {
	n := strings.ToLower(notice)
	switch {
	case strings.Contains(n, "fail"),
		strings.Contains(n, "error"),
		strings.Contains(n, "invalid"),
		strings.Contains(n, "denied"),
		strings.Contains(n, "rate limit"),
		strings.Contains(n, "timeout"),
		strings.Contains(n, "unauthorized"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	case strings.HasPrefix(notice, "Refreshed"),
		strings.HasPrefix(notice, "Eyrie:"),
		strings.Contains(notice, "Removed API key"),
		strings.Contains(notice, "Setup complete"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6BCB77"))
	default:
		return configMutedStyle()
	}
}

func renderConfigNotice(notice string) string {
	notice = sanitizeConfigNotice(notice)
	if notice == "" {
		return ""
	}
	return configNoticeStyle(notice).Render(notice)
}

func (m chatModel) configHelpLine() string {
	muted := configMutedStyle()
	if m.configSaving {
		return muted.Render(m.spinner.View() + " working…")
	}
	return muted.Render("←/→ tabs · ↑/↓ · enter · esc close")
}
