package cmd

import (
	"context"
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

func (m chatModel) configKeysCredentialIndex(provider string) int {
	for i, row := range m.configKeysRows(hawkconfig.ConfiguredCredentialProviders()) {
		if row.kind == configKeysRowCredential && row.provider == provider {
			return i
		}
	}
	return -1
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

	if len(tableRows) > 0 {
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
	switch {
	case m.configKeysPendingRemove != "":
		name := hawkconfig.GatewayDisplayName(m.configKeysPendingRemove)
		b.WriteString("\n" + mutedStyle.Render(indent+configKeysRemovePrompt(m.configKeysRemoveStep, name)))
	case len(configured) > 0:
		b.WriteString("\n" + mutedStyle.Render(indent+"enter open key · delete remove · stored in "+credentialsStoreLabel()))
	default:
		b.WriteString("\n" + mutedStyle.Render(indent+"enter Add API key to paste · stored in "+credentialsStoreLabel()))
	}
	return m.configTabShellView(b.String())
}

func (m chatModel) configKeyDetailView() string {
	mutedStyle := configMutedStyle()
	accentStyle := configAccentStyle()
	activeStyle := configActiveStyle()
	provider := strings.TrimSpace(m.configProvider)
	displayName := hawkconfig.GatewayDisplayName(provider)
	masked := hawkconfig.MaskCredentialForProvider(context.Background(), provider)

	var b strings.Builder
	b.WriteString(renderConfigBreadcrumb(displayName+" key") + "\n\n")
	b.WriteString(mutedStyle.Render("  Gateway: ") + accentStyle.Render(displayName) + "\n")
	b.WriteString(mutedStyle.Render("  Key: ") + activeStyle.Render(masked) + "\n")
	b.WriteString(mutedStyle.Render("  Stored in: "+credentialsStoreLabel()) + "\n")
	return m.configTabShellView(b.String())
}

func credentialsStoreLabel() string {
	return credentials.PlatformSecretStoreName()
}

func configKeysRemovePrompt(step int, gatewayName string) string {
	switch step {
	case 2:
		return fmt.Sprintf("This permanently removes the key for %s — press enter again to confirm · esc cancel", gatewayName)
	default:
		return fmt.Sprintf("Remove key for %s? press enter to continue · esc cancel", gatewayName)
	}
}

func configKeysRemoveNotice(step int, gatewayName string) string {
	switch step {
	case 2:
		return fmt.Sprintf("Press enter again to permanently remove the key for %s · esc cancel", gatewayName)
	default:
		return fmt.Sprintf("Remove key for %s? Press enter to continue · esc cancel", gatewayName)
	}
}

func (m chatModel) startConfigKeyView(provider string) chatModel {
	m.configEntry = configEntryKeyView
	m.configProvider = provider
	m.configKeysPendingRemove = ""
	m.configKeysRemoveStep = 0
	m.configNotice = ""
	return m
}

func (m chatModel) startConfigKeyReplace(provider string) (chatModel, tea.Cmd) {
	m.configReplaceProvider = provider
	m.configEntry = configEntryNone
	m.configNotice = "Paste replacement API key for " + hawkconfig.GatewayDisplayName(provider)
	return m.startConfigEntry(configEntryAPIKeyPaste, provider)
}

func (m chatModel) beginConfigKeysRemove(provider string) chatModel {
	m.configKeysPendingRemove = provider
	m.configKeysRemoveStep = 1
	name := hawkconfig.GatewayDisplayName(provider)
	m.configNotice = configKeysRemoveNotice(1, name)
	return m
}

func (m chatModel) clearConfigKeysPendingRemove() chatModel {
	m.configKeysPendingRemove = ""
	m.configKeysRemoveStep = 0
	if strings.Contains(m.configNotice, "Remove key for") ||
		strings.Contains(m.configNotice, "Press enter to continue") ||
		strings.Contains(m.configNotice, "permanently remove") {
		m.configNotice = ""
	}
	return m
}

func (m chatModel) advanceConfigKeysRemove() (chatModel, tea.Cmd) {
	provider := strings.TrimSpace(m.configKeysPendingRemove)
	if provider == "" {
		return m, nil
	}
	if m.configKeysRemoveStep < 2 {
		m.configKeysRemoveStep = 2
		name := hawkconfig.GatewayDisplayName(provider)
		m.configNotice = configKeysRemoveNotice(2, name)
		return m, nil
	}
	return m.confirmConfigKeysRemove()
}

func (m chatModel) confirmConfigKeysRemove() (chatModel, tea.Cmd) {
	provider := strings.TrimSpace(m.configKeysPendingRemove)
	if provider == "" {
		return m, nil
	}
	m.configKeysPendingRemove = ""
	m.configKeysRemoveStep = 0
	m.configSaving = true
	m.configNotice = fmt.Sprintf("Removing key for %s…", hawkconfig.GatewayDisplayName(provider))
	if m.configEntry == configEntryKeyView {
		m.configEntry = configEntryNone
		m.configProvider = ""
	}
	return m, removeCredentialAsync(provider)
}

func (m chatModel) selectedConfigKeysCredential() (configKeysRow, bool) {
	rows := m.configKeysRows(hawkconfig.ConfiguredCredentialProviders())
	if m.configSel < 0 || m.configSel >= len(rows) {
		return configKeysRow{}, false
	}
	row := rows[m.configSel]
	if row.kind != configKeysRowCredential {
		return configKeysRow{}, false
	}
	return row, true
}

func (m chatModel) handleConfigKeysDelete() chatModel {
	if row, ok := m.selectedConfigKeysCredential(); ok {
		return m.beginConfigKeysRemove(row.provider)
	}
	return m
}

func (m chatModel) handleConfigKeysSelect() (chatModel, tea.Cmd) {
	if m.configKeysPendingRemove != "" {
		return m.advanceConfigKeysRemove()
	}
	rows := m.configKeysRows(hawkconfig.ConfiguredCredentialProviders())
	if m.configSel < 0 || m.configSel >= len(rows) {
		return m, nil
	}
	row := rows[m.configSel]
	switch row.kind {
	case configKeysRowCredential:
		return m.startConfigKeyView(row.provider), nil
	case configKeysActionAdd:
		m.configNotice = "Paste your API key"
		return m.startConfigEntry(configEntryAPIKeyPaste, "")
	case configKeysActionOllama:
		return m.startConfigOllamaURL()
	default:
		return m, nil
	}
}

func (m chatModel) handleConfigKeyViewKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	provider := strings.TrimSpace(m.configProvider)
	switch msg.Type {
	case tea.KeyEsc:
		m.configEntry = configEntryNone
		m.configProvider = ""
		m = m.clearConfigKeysPendingRemove()
		if idx := m.configKeysCredentialIndex(provider); idx >= 0 {
			m.configSel = idx
		}
		return m, nil
	case tea.KeyDelete, tea.KeyBackspace:
		if provider == "" {
			return m, nil
		}
		return m.beginConfigKeysRemove(provider), nil
	case tea.KeyEnter:
		if m.configKeysPendingRemove != "" {
			return m.advanceConfigKeysRemove()
		}
		if provider == "" {
			return m, nil
		}
		return m.startConfigKeyReplace(provider)
	default:
		return m, nil
	}
}

func (m chatModel) handleConfigKeysEsc() chatModel {
	if m.configKeysPendingRemove != "" {
		return m.clearConfigKeysPendingRemove()
	}
	return m.closeConfigPanel()
}
