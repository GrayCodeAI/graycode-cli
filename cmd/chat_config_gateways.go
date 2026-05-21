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
			modelCacheMu.RLock()
			if cached, ok := modelCache[id]; ok {
				count = len(cached)
			}
			modelCacheMu.RUnlock()
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
	cursorStyle := configSelectedStyle()
	rowStyle := configRowStyle()
	activeStyle := configActiveStyle()
	mutedStyle := configMutedStyle()
	headerStyle := configHeaderStyle()
	metaStyle := configMutedStyle()

	rows := m.configGatewayRows()

	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+configWindowSize {
		m.configScroll = m.configSel - configWindowSize + 1
	}

	headers := []string{"Gateway", "Key", "Catalog", "Active"}
	tableData := make([][]string, len(rows))
	layoutData := make([][]string, len(rows))
	for i, row := range rows {
		key := "—"
		if row.HasKey {
			key = "✓"
		}
		models := "—"
		if row.ModelCount > 0 {
			models = fmt.Sprintf("%d", row.ModelCount)
		}
		tableData[i] = []string{row.DisplayName, key, models, ""}
		layoutData[i] = append([]string(nil), tableData[i]...)
		if row.Active {
			layoutData[i][3] = "●"
		}
	}
	layout := computeConfigTableLayout(m.configPanelViewWidth(), headers, layoutData, []int{2, 2, 2, 2}, true)

	var b strings.Builder
	b.WriteString(renderConfigTableHeader(headers, layout, headerStyle, metaStyle) + "\n")

	if m.configScroll > 0 {
		b.WriteString(configTableScrollHint(m.configScroll, 0, mutedStyle) + "\n")
	}
	end := m.configScroll + configWindowSize
	if end > len(rows) {
		end = len(rows)
	}
	for i := m.configScroll; i < end; i++ {
		row := rows[i]
		b.WriteString(renderConfigTableRow(
			tableData[i],
			i == m.configSel,
			row.Active,
			true,
			layout,
			rowStyle,
			cursorStyle,
			activeStyle,
			metaStyle,
		) + "\n")
	}
	if end < len(rows) {
		b.WriteString(configTableScrollHint(0, len(rows)-end, mutedStyle) + "\n")
	}

	b.WriteString("\n")
	refreshSel := len(rows)
	refreshHint := "Refresh gateway"
	if m.configGatewayFocus >= 0 && m.configGatewayFocus < len(rows) {
		refreshHint = "Refresh " + rows[m.configGatewayFocus].DisplayName
	}
	b.WriteString(renderConfigTableActionRow(refreshHint, m.configSel == refreshSel, rowStyle, cursorStyle) + "\n")

	ctx := context.Background()
	indent := strings.Repeat(" ", configTableIndent)
	if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		b.WriteString("\n" + mutedStyle.Render(indent+"Catalog = models in eyrie cache · add key in Keys tab to use them"))
	} else {
		b.WriteString("\n" + configTableSelectionFooter(len(rows), m.configScroll, end, mutedStyle, "enter select · ↓ refresh row"))
	}
	return m.configTabShellView(b.String())
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
