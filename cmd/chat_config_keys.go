package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/eyrie/credentials"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

type configKeysRow struct {
	kind     string // configKeysRowCredential, configKeysActionAdd, configKeysActionOllama
	provider string
}

func (m chatModel) configKeysRows(configured []string) []configKeysRow {
	var rows []configKeysRow
	for _, p := range configured {
		rows = append(rows, configKeysRow{kind: configKeysRowCredential, provider: p})
	}
	rows = append(
		rows,
		configKeysRow{kind: configKeysActionAdd},
		configKeysRow{kind: configKeysActionOllama},
	)
	return rows
}

func (m chatModel) configKeysAddRowIndex() int {
	return len(hawkconfig.ConfiguredCredentialProviders())
}

func (m chatModel) configKeysView() string {
	selectedStyle := configSelectedStyle()
	rowStyle := configRowStyle()
	mutedStyle := configMutedStyle()

	configured := hawkconfig.ConfiguredCredentialProviders()
	rows := m.configKeysRows(configured)
	var b strings.Builder
	if len(configured) == 0 {
		b.WriteString(mutedStyle.Render("  No API keys yet — select Add API key below, press enter, paste") + "\n\n")
	}
	b.WriteString(padKeysTable("Gateway", "Status", 20, 12) + "\n")
	for i, row := range rows {
		prefix := "  "
		style := rowStyle
		if i == m.configSel {
			prefix = "❯ "
			style = selectedStyle
		}
		switch row.kind {
		case configKeysRowCredential:
			name := hawkconfig.GatewayDisplayName(row.provider)
			b.WriteString(style.Render(prefix+padKeysTable(name, "✓ saved", 20, 12)) + "\n")
		case configKeysActionAdd:
			b.WriteString("\n" + style.Render(prefix+"Add API key") + "\n")
		case configKeysActionOllama:
			b.WriteString(style.Render(prefix+"Ollama URL (local)") + "\n")
		}
	}
	if len(configured) > 0 {
		b.WriteString(mutedStyle.Render("\nenter saved row to remove key") + "\n")
	} else {
		b.WriteString(mutedStyle.Render("\nenter Add API key to paste · stored in "+credentialsStoreLabel()) + "\n")
	}
	return m.configTabShellView(b.String())
}

func credentialsStoreLabel() string {
	return credentials.PlatformSecretStoreName()
}

func (m chatModel) handleConfigKeysSelect() (chatModel, tea.Cmd) {
	rows := m.configKeysRows(hawkconfig.ConfiguredCredentialProviders())
	if m.configSel < 0 || m.configSel >= len(rows) {
		return m, nil
	}
	row := rows[m.configSel]
	switch row.kind {
	case configKeysRowCredential:
		m.configSaving = true
		m.configNotice = fmt.Sprintf("Removing key for %s…", hawkconfig.GatewayDisplayName(row.provider))
		return m, removeCredentialAsync(row.provider)
	case configKeysActionAdd:
		m.configNotice = "Paste your API key"
		return m.startConfigEntry(configEntryAPIKeyPaste, "")
	case configKeysActionOllama:
		return m.startConfigOllamaURL()
	default:
		return m, nil
	}
}

func padKeysTable(c1, c2 string, w1, w2 int) string {
	return fmt.Sprintf("%-*s %-*s", w1, truncateRunes(c1, w1), w2, truncateRunes(c2, w2))
}

func (m chatModel) handleConfigKeysEsc() chatModel {
	return m.closeConfigPanel()
}
