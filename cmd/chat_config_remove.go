package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

type configRemoveCredentialMsg struct {
	provider string
	removed  []string
	err      error
}

func (m chatModel) configRemoveKeyLabels() []string {
	providers := hawkconfig.ConfiguredCredentialProviders()
	out := make([]string, len(providers))
	for i, p := range providers {
		out[i] = p
	}
	return out
}

func (m chatModel) beginConfigRemoveKeyPicker() (chatModel, tea.Cmd) {
	providers := hawkconfig.ConfiguredCredentialProviders()
	if len(providers) == 0 {
		m.configMenu = "hub"
		m.configNotice = "No stored API keys to remove"
		return m, nil
	}
	m.configMenu = "remove-key"
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = "Select provider to remove its API key from the OS secret store"
	return m, nil
}

func (m chatModel) configRemoveKeyView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	opts := m.configRemoveKeyLabels()
	total := len(opts)
	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+configWindowSize {
		m.configScroll = m.configSel - configWindowSize + 1
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("🗑 Remove API key") + "\n\n")
	if notice := strings.TrimSpace(m.configNotice); notice != "" {
		b.WriteString(mutedStyle.Render(notice) + "\n\n")
	}
	if total == 0 {
		b.WriteString(mutedStyle.Render("  No stored API keys.") + "\n")
		b.WriteString("\n" + mutedStyle.Render("esc → back"))
		return b.String()
	}
	end := m.configScroll + configWindowSize
	if end > total {
		end = total
	}
	for i := m.configScroll; i < end; i++ {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		b.WriteString(lineStyle.Render(prefix+opts[i]) + "\n")
	}
	help := "↑/↓ · enter remove · esc back"
	if m.configSaving {
		help = "please wait…"
	}
	b.WriteString("\n" + mutedStyle.Render(help))
	return b.String()
}

func (m chatModel) handleConfigRemoveKeySelect() (chatModel, tea.Cmd) {
	if m.configSaving {
		return m, nil
	}
	providers := hawkconfig.ConfiguredCredentialProviders()
	if m.configSel < 0 || m.configSel >= len(providers) {
		return m, nil
	}
	provider := providers[m.configSel]
	m.configSaving = true
	m.configNotice = fmt.Sprintf("Removing API key for %s…", provider)
	return m, removeCredentialAsync(provider)
}

func removeCredentialAsync(provider string) tea.Cmd {
	return func() tea.Msg {
		removed, err := hawkconfig.RemoveStoredCredential(context.Background(), provider)
		return configRemoveCredentialMsg{
			provider: provider,
			removed:  removed,
			err:      err,
		}
	}
}

func (m chatModel) handleConfigRemoveCredentialMsg(msg configRemoveCredentialMsg) (chatModel, tea.Cmd) {
	m.configSaving = false
	if msg.err != nil {
		m.configNotice = msg.err.Error()
		m.configMenu = "remove-key"
		return m, nil
	}
	delete(modelCache, msg.provider)
	m.configMenu = "hub"
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = fmt.Sprintf("Removed API key for %s (%s)", msg.provider, strings.Join(msg.removed, ", "))
	next, cmd := m.rebuildSessionTransport()
	next.configNotice = next.configHubNotice() + "\n" + fmt.Sprintf("Removed key for %s", msg.provider)
	return next, cmd
}

func (m chatModel) openConfigRemoveKeyPanel() (chatModel, tea.Cmd) {
	next, cmd := m.openConfigPanel()
	next, _ = next.beginConfigRemoveKeyPicker()
	return next, cmd
}
