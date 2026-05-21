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
	cursorStyle := configSelectedStyle()
	rowStyle := configRowStyle()
	activeStyle := configActiveStyle()
	mutedStyle := configMutedStyle()
	headerStyle := configHeaderStyle()
	metaStyle := configMutedStyle()

	configured := hawkconfig.ConfiguredCredentialProviders()
	rows := m.configKeysRows(configured)
	indent := strings.Repeat(" ", configTableIndent)

	var b strings.Builder
	if len(configured) == 0 {
		b.WriteString(mutedStyle.Render(indent + "No API keys yet — select Add API key below, press enter, paste") + "\n\n")
	}

	headers := []string{"Gateway", "Status"}
	tableRows := make([][]string, 0, len(configured))
	for _, row := range rows {
		if row.kind != configKeysRowCredential {
			continue
		}
		tableRows = append(tableRows, []string{
			hawkconfig.GatewayDisplayName(row.provider),
			"✓ saved",
		})
	}
	layout := computeConfigTableLayout(m.configPanelViewWidth(), headers, tableRows, []int{4, 4}, false)

	b.WriteString(renderConfigTableHeader(headers, layout, headerStyle, metaStyle) + "\n")
	credIdx := 0
	for i, row := range rows {
		if row.kind != configKeysRowCredential {
			continue
		}
		b.WriteString(renderConfigTableRow(
			tableRows[credIdx],
			i == m.configSel,
			false,
			false,
			layout,
			rowStyle,
			cursorStyle,
			activeStyle,
			metaStyle,
		) + "\n")
		credIdx++
	}

	b.WriteString("\n")
	for i, row := range rows {
		switch row.kind {
		case configKeysActionAdd:
			b.WriteString(renderConfigTableActionRow("Add API key", i == m.configSel, rowStyle, cursorStyle) + "\n")
		case configKeysActionOllama:
			b.WriteString(renderConfigTableActionRow("Ollama URL (local)", i == m.configSel, rowStyle, cursorStyle) + "\n")
		}
	}
	if len(configured) > 0 {
		b.WriteString("\n" + mutedStyle.Render(indent+"enter saved row to remove key"))
	} else {
		b.WriteString("\n" + mutedStyle.Render(indent+"enter Add API key to paste · stored in "+credentialsStoreLabel()))
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

func (m chatModel) handleConfigKeysEsc() chatModel {
	return m.closeConfigPanel()
}
