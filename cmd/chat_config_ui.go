package cmd

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func configMutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(textMuted)
}

func configTitleStyle() lipgloss.Style {
	// Brand orange — title is the voice of the config panel.
	return lipgloss.NewStyle().Foreground(hawkColor).Bold(true)
}

func configSelectedStyle() lipgloss.Style {
	// Hot pink — active selection. Distinct from the title (orange) so
	// the user can see which item is the focus without confusing the
	// panel voice with the focus indicator.
	return lipgloss.NewStyle().Foreground(activePink).Bold(true)
}

func configAccentStyle() lipgloss.Style {
	// Accent for inline highlights (breadcrumb, status line values).
	// Same as title — both are the panel's voice.
	return lipgloss.NewStyle().Foreground(hawkColor)
}

func renderConfigBreadcrumb(title string) string {
	muted := configMutedStyle().Inline(true)
	accent := configAccentStyle().Inline(true)
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		muted.Render("Keys › "),
		accent.Render(title),
	)
}

func renderConfigGatewayLine(displayName string) string {
	indent := strings.Repeat(" ", modelTableIndent)
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		configMutedStyle().Inline(true).Render(indent+"Gateway: "),
		configAccentStyle().Inline(true).Render(displayName),
	)
}

func renderConfigStatusLine(m chatModel) string {
	gateway, model, configured := m.configStatus()
	muted := configMutedStyle().Inline(true)
	accent := configAccentStyle().Inline(true)
	active := configActiveStyle().Inline(true)

	if !configured {
		return lipgloss.JoinHorizontal(
			lipgloss.Left,
			muted.Render("Gateway: "),
			muted.Render("none"),
			muted.Render(" · no API key — add one in "),
			accent.Render("Keys"),
		)
	}

	gatewayStyle := accent
	if gateway == "none" {
		gatewayStyle = muted
	}
	parts := []string{
		muted.Render("Gateway: "),
		gatewayStyle.Render(gateway),
	}
	if model == "" {
		parts = append(parts, muted.Render(" · no model selected"))
	} else {
		parts = append(parts, muted.Render(" · Model: "), active.Render(model))
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func configActiveStyle() lipgloss.Style {
	// The current gateway/model name in the status line — pink to match
	// the active-selection color so the "current" indicator and the
	// "focus" indicator share a visual language.
	return lipgloss.NewStyle().Foreground(activePink)
}

func configRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(textPrimary)
}

func configHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(textPrimary).Bold(true)
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
		return lipgloss.NewStyle().Foreground(errorCoral)
	case strings.HasPrefix(notice, "Refreshed"),
		strings.HasPrefix(notice, "Eyrie:"),
		strings.Contains(notice, "Removed API key"),
		strings.Contains(notice, "Setup complete"):
		return lipgloss.NewStyle().Foreground(successTeal)
	default:
		return configMutedStyle()
	}
}

func renderConfigRefreshActionRow(gatewayName string, cursor bool) string {
	muted := configMutedStyle().Inline(true)
	accent := configAccentStyle().Inline(true)
	cursorStyle := configSelectedStyle().Inline(true)
	prefix := strings.Repeat(" ", configTableIndent)
	if cursor {
		prefix = strings.Repeat(" ", configTableIndent-2) + cursorStyle.Render(iconPrompt) + " "
		return prefix + cursorStyle.Render("Refresh "+gatewayName)
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		muted.Render(prefix+"Refresh "),
		accent.Render(gatewayName),
	)
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
	if m.configEntry == configEntryKeyView {
		if m.configKeysPendingRemove != "" {
			if m.configKeysRemoveStep >= 2 {
				return muted.Render("enter again to permanently remove · esc cancel")
			}
			return muted.Render("enter continue · esc cancel")
		}
		return muted.Render("enter replace key · delete remove · esc back")
	}
	if m.configTab == configTabKeys && m.configKeysPendingRemove != "" {
		if m.configKeysRemoveStep >= 2 {
			return muted.Render("enter again to permanently remove · esc cancel")
		}
		return muted.Render("enter continue · esc cancel")
	}
	if m.configTab == configTabModels {
		if m.configModelSearchActive {
			return muted.Render("↑/↓ navigate · enter select · esc clear search")
		}
		return muted.Render("↑/↓ navigate · enter select · / search · esc close")
	}
	return muted.Render("←/→ tabs · ↑/↓ · enter · esc close")
}
