package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
)

type configGatewayRow struct {
	ID          string
	DisplayName string
	HasKey      bool
	ModelCount  int
	Active      bool
}

type configGatewayRefreshMsg struct {
	providerID string
	summary    string
	err        error
}

func (m chatModel) configGatewayRows() []configGatewayRow {
	providers := hawkconfig.AllSetupGateways()
	configured := configuredGatewayKeys()
	active := strings.TrimSpace(m.configModelProvider)
	if active == "" && m.session != nil {
		active = strings.TrimSpace(m.session.Provider())
	}
	var rows []configGatewayRow
	for _, id := range providers {
		if id == "" {
			continue
		}
		count := hawkconfig.CachedModelCountForProvider(id)
		if count == 0 {
			if cached, ok := modelCache[id]; ok {
				count = len(cached)
			}
		}
		rows = append(rows, configGatewayRow{
			ID:          id,
			DisplayName: hawkconfig.GatewayDisplayName(id),
			HasKey:      configured[id] || id == configProviderOllama && configured[configProviderOllama],
			ModelCount:  count,
			Active:      hawkconfig.NormalizeProviderForEngine(id) == hawkconfig.NormalizeProviderForEngine(active),
		})
	}
	return rows
}

func (m chatModel) configGatewaysView() string {
	selectedStyle := configSelectedStyle()
	rowStyle := configRowStyle()
	mutedStyle := configMutedStyle()
	headerStyle := configHeaderStyle()

	rows := m.configGatewayRows()

	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+configWindowSize {
		m.configScroll = m.configSel - configWindowSize + 1
	}

	var b strings.Builder
	b.WriteString("  " + headerStyle.Render(padGatewayTable("Gateway", "Key", "Catalog", "Active", 14, 6, 8, 8)) + "\n")
	if m.configScroll > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more above ···", m.configScroll)) + "\n")
	}
	end := m.configScroll + configWindowSize
	if end > len(rows) {
		end = len(rows)
	}
	for i := m.configScroll; i < end; i++ {
		row := rows[i]
		prefix := "  "
		style := rowStyle
		if i == m.configSel {
			prefix = "❯ "
			style = selectedStyle
		}
		key := "—"
		if row.HasKey {
			key = "✓"
		}
		active := ""
		if row.Active {
			active = "●"
		}
		models := "—"
		if row.ModelCount > 0 {
			models = fmt.Sprintf("%d", row.ModelCount)
		}
		line := padGatewayTable(row.DisplayName, key, models, active, 14, 6, 8, 8)
		b.WriteString(style.Render(prefix+line) + "\n")
	}
	if end < len(rows) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more below ···", len(rows)-end)) + "\n")
	}
	b.WriteString("\n")
	refreshSel := len(rows)
	prefix := "  "
	style := rowStyle
	if m.configSel == refreshSel {
		prefix = "❯ "
		style = selectedStyle
	}
	refreshHint := "Refresh gateway"
	if m.configGatewayFocus >= 0 && m.configGatewayFocus < len(rows) {
		refreshHint = "Refresh " + rows[m.configGatewayFocus].DisplayName
	}
	b.WriteString(style.Render(prefix+refreshHint) + "\n")
	ctx := context.Background()
	if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		b.WriteString(mutedStyle.Render("\nCatalog = models in eyrie cache · add key in Keys tab to use them"))
	} else {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("\n%d gateways · enter select · ↓ refresh row", len(rows))))
	}
	return m.configTabShellView(b.String())
}

func padGatewayTable(c1, c2, c3, c4 string, w1, w2, w3, w4 int) string {
	return fmt.Sprintf("%-*s %-*s %-*s %-*s", w1, truncateRunes(c1, w1), w2, truncateRunes(c2, w2), w3, truncateRunes(c3, w3), w4, truncateRunes(c4, w4))
}

func (m chatModel) handleConfigGatewaysSelect() (chatModel, tea.Cmd) {
	rows := m.configGatewayRows()
	refreshIdx := len(rows)
	if m.configSel == refreshIdx {
		if len(rows) == 0 {
			m.configNotice = "No gateways available"
			return m, nil
		}
		idx := m.configGatewayFocus
		if idx < 0 || idx >= len(rows) {
			idx = 0
		}
		gw := rows[idx].ID
		m.configSaving = true
		m.configNotice = "Refreshing " + gw + "…"
		return m, refreshGatewayAsync(gw)
	}
	if m.configSel < 0 || m.configSel >= len(rows) {
		return m, nil
	}
	row := rows[m.configSel]
	if !row.HasKey {
		if row.ID == configProviderOllama {
			return m.startConfigOllamaURL()
		}
		m.configTab = configTabKeys
		m.configSel = m.configKeysAddRowIndex()
		m.configNotice = fmt.Sprintf("Add an API key for %s first — Keys tab → Add API key", row.DisplayName)
		return m, nil
	}
	gw := row.ID
	m.configGatewayFocus = m.configSel
	m.configModelProvider = gw
	_ = hawkconfig.SetGlobalSetting("provider", gw)
	m.session.SetProvider(hawkconfig.NormalizeProviderForEngine(gw))
	m.configTab = configTabModels
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = "Gateway: " + gw
	return m.beginConfigModelsTab()
}

func refreshGatewayAsync(providerID string) tea.Cmd {
	return func() tea.Msg {
		summary, err := hawkconfig.RefreshGatewayCatalog(context.Background(), providerID)
		return configGatewayRefreshMsg{providerID: providerID, summary: summary, err: err}
	}
}

func (m chatModel) handleConfigGatewayRefreshMsg(msg configGatewayRefreshMsg) chatModel {
	m.configSaving = false
	InvalidateModelCacheProvider(msg.providerID)
	if msg.err != nil {
		m.configNotice = sanitizeConfigNotice(eyrieclient.FormatSetupError(msg.providerID, msg.err))
		return m
	}
	m.configNotice = msg.summary
	if m.configTab == configTabModels && strings.TrimSpace(m.configModelProvider) == msg.providerID {
		m.configModelOptions = loadConfigModelOptions(msg.providerID)
	}
	return m
}

func (m chatModel) trackConfigGatewayFocus() chatModel {
	if m.configTab != configTabGateways {
		return m
	}
	rows := len(hawkconfig.AllSetupGateways())
	if m.configSel >= 0 && m.configSel < rows {
		m.configGatewayFocus = m.configSel
	}
	return m
}
