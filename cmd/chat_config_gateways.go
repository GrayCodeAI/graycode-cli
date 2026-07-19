package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

type configGatewayRow struct {
	ID             string
	DisplayName    string
	HasKey         bool
	Configured     bool
	ModelCount     int
	Active         bool
	RegionLabel    string
	RegionRequired bool
	CredentialEnv  string
	KeyConflict    bool
}

type configGatewayRefreshMsg struct {
	providerID string
	summary    string
	err        error
}

func (m chatModel) configGatewayRows() []configGatewayRow {
	if !m.configGatewayRowsDirty && m.configGatewayRowsCache != nil {
		return m.configGatewayRowsCache
	}
	return m.loadConfigGatewayRows()
}

func (m chatModel) loadConfigGatewayRows() []configGatewayRow {
	ctx := context.Background()
	active := strings.TrimSpace(m.configModelProvider)
	activeModel := ""
	if active == "" && m.session != nil {
		active = strings.TrimSpace(m.session.Provider())
	}
	if m.session != nil {
		activeModel = strings.TrimSpace(m.session.Model())
	}
	statuses := hawkconfig.GatewayStatuses(ctx, active, activeModel)
	rows := make([]configGatewayRow, 0, len(statuses))
	for _, status := range statuses {
		if status.ID == "" {
			continue
		}
		count := status.ModelCount
		if count == 0 {
			modelCacheMu.RLock()
			if cached, ok := modelCache[status.ID]; ok {
				count = len(cached)
			}
			modelCacheMu.RUnlock()
		}
		hasKey := status.HasStoredCredential
		credentialEnv, keyConflict := "", false
		if hasKey {
			credentialEnv, keyConflict = hawkconfig.CredentialEnvironmentConflict(ctx, status.ID)
		}
		display := status.DisplayName
		if status.ID == hawkconfig.ProviderXiaomiTokenPlan {
			if reg := status.RegionLabel; reg != "" {
				display += " · " + reg
			} else {
				display += " · region required"
			}
		}
		if status.ID == hawkconfig.ProviderZAICoding {
			if reg := status.RegionLabel; reg != "" {
				display += " · " + reg
			} else {
				display += " · region"
			}
		}
		rows = append(rows, configGatewayRow{
			ID:             status.ID,
			DisplayName:    display,
			HasKey:         hasKey,
			Configured:     status.HasConfiguredDeployment || hasKey,
			ModelCount:     count,
			Active:         status.Active || hawkconfig.ActiveProviderID(status.ID) == hawkconfig.ActiveProviderID(active),
			RegionLabel:    status.RegionLabel,
			RegionRequired: status.RegionRequired,
			CredentialEnv:  credentialEnv,
			KeyConflict:    keyConflict,
		})
	}
	return prioritizeConfigGatewayRows(rows)
}

func prioritizeConfigGatewayRows(rows []configGatewayRow) []configGatewayRow {
	ordered := make([]configGatewayRow, 0, len(rows))
	for _, row := range rows {
		if row.Active {
			ordered = append(ordered, row)
		}
	}
	for _, row := range rows {
		if !row.Active && row.Configured {
			ordered = append(ordered, row)
		}
	}
	for _, row := range rows {
		if !row.Active && !row.Configured {
			ordered = append(ordered, row)
		}
	}
	return ordered
}

func (m chatModel) refreshConfigGatewayRows() chatModel {
	selectedID := ""
	focusID := ""
	if m.configSel >= 0 && m.configSel < len(m.configGatewayRowsCache) {
		selectedID = m.configGatewayRowsCache[m.configSel].ID
	}
	if m.configGatewayFocus >= 0 && m.configGatewayFocus < len(m.configGatewayRowsCache) {
		focusID = m.configGatewayRowsCache[m.configGatewayFocus].ID
	}
	m.configGatewayRowsCache = m.loadConfigGatewayRows()
	m.configGatewayRowsDirty = false
	for i, row := range m.configGatewayRowsCache {
		if row.ID == selectedID {
			m.configSel = i
		}
		if row.ID == focusID {
			m.configGatewayFocus = i
		}
	}
	return m
}

func (m chatModel) invalidateConfigGatewayRows() chatModel {
	m.configGatewayRowsDirty = true
	return m
}

func (m chatModel) configGatewayRowIndex(provider string) int {
	provider = strings.TrimSpace(provider)
	for i, row := range m.configGatewayRows() {
		if row.ID == provider {
			return i
		}
	}
	return -1
}

func (m chatModel) activeGatewayRowIndex(rows []configGatewayRow) int {
	for i, row := range rows {
		if row.Active {
			return i
		}
	}
	return -1
}

// configGatewayRefreshTargetIndex is the gateway row used for refresh label and action.
func (m chatModel) configGatewayRefreshTargetIndex(rows []configGatewayRow) int {
	if len(rows) == 0 {
		return 0
	}
	if m.configSel >= 0 && m.configSel < len(rows) {
		return m.configSel
	}
	if m.configGatewayFocus >= 0 && m.configGatewayFocus < len(rows) {
		return m.configGatewayFocus
	}
	if i := m.activeGatewayRowIndex(rows); i >= 0 {
		return i
	}
	return 0
}

func (m chatModel) refreshConfigGateway() (chatModel, tea.Cmd) {
	rows := m.configGatewayRows()
	if len(rows) == 0 {
		m.configNotice = "No gateways available"
		return m, nil
	}
	idx := m.configGatewayRefreshTargetIndex(rows)
	row := rows[idx]
	if row.ID == hawkconfig.ProviderXiaomiTokenPlan && row.RegionRequired {
		m.configNotice = "Pick Token Plan region (cn / sgp / ams) before refresh"
		return m.startConfigXiaomiTokenPlanRegion(), nil
	}
	if row.ID == hawkconfig.ProviderZAICoding && row.RegionRequired {
		m.configNotice = "Pick Coding Plan region (international / cn) before refresh"
		return m.startConfigZAIRegion(row.ID), nil
	}

	if !row.HasKey {
		m.configNotice = fmt.Sprintf("Select %s and press enter to paste an API key", row.DisplayName)
		return m, nil
	}
	m.configSaving = true
	m.configNotice = "Refreshing " + row.DisplayName + "…"
	return m, refreshGatewayAsync(row.ID)
}

func (m chatModel) configGatewaysView() string {
	cursorStyle := configSelectedStyle()
	rowStyle := configRowStyle()
	activeStyle := configActiveStyle()
	mutedStyle := configMutedStyle()
	headerStyle := configHeaderStyle()
	metaStyle := configMutedStyle()

	rows := m.configGatewayRows()
	refreshSel := len(rows)
	windowSize := m.configVisibleRows()
	maxScroll := maxInt(0, len(rows)-windowSize)
	if m.configScroll > maxScroll {
		m.configScroll = maxScroll
	}

	if m.configSel < len(rows) {
		if m.configSel < m.configScroll {
			m.configScroll = m.configSel
		}
		if m.configSel >= m.configScroll+windowSize {
			m.configScroll = m.configSel - windowSize + 1
		}
	}

	headers := []string{"Gateway", "Key", "Catalog", "Active"}
	tableData := make([][]string, len(rows))
	layoutData := make([][]string, len(rows))
	for i, row := range rows {
		key := "—"
		if row.HasKey {
			key = "+" + icons.CheckBold() + " "
			if row.KeyConflict {
				key = "+" + icons.CheckBold() + " !"
			}
		}
		models := "key required"
		if row.HasKey && row.ModelCount > 0 {
			models = fmt.Sprintf("%d", row.ModelCount)
		} else if row.HasKey {
			models = "—"
		}
		tableData[i] = []string{row.DisplayName, key, models, ""}
		layoutData[i] = append([]string(nil), tableData[i]...)
		if row.Active {
			layoutData[i][3] = icons.CircleFilled()
		}
	}
	layout := computeConfigTableLayout(m.configPanelViewWidth(), headers, layoutData, []int{2, 2, 2, 2}, true)

	var b strings.Builder
	b.WriteString(renderConfigTableHeader(headers, layout, headerStyle, metaStyle) + "\n")

	if m.configScroll > 0 {
		b.WriteString(configTableScrollHint(m.configScroll, 0, mutedStyle) + "\n")
	}
	end := m.configScroll + windowSize
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
	targetIdx := m.configGatewayRefreshTargetIndex(rows)
	b.WriteString(renderConfigRefreshActionRow(rows[targetIdx].DisplayName, m.configSel == refreshSel) + "\n")
	if targetIdx >= 0 && targetIdx < len(rows) && rows[targetIdx].KeyConflict {
		warning := fmt.Sprintf("warning: %s differs from the stored keychain credential", rows[targetIdx].CredentialEnv)
		b.WriteString("\n" + configWarningStyle().Render(strings.Repeat(" ", configTableIndent)+warning))
	}

	ctx := context.Background()
	indent := strings.Repeat(" ", configTableIndent)
	if m.configKeysPendingRemove != "" {
		name := hawkconfig.GatewayDisplayName(m.configKeysPendingRemove)
		b.WriteString("\n" + mutedStyle.Render(indent+configGatewayRemovePrompt(m.configKeysRemoveStep, name)))
	} else if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		hint := "Select a gateway · enter · paste API key · then Models tab"
		if targetIdx >= 0 && targetIdx < len(rows) && rows[targetIdx].ID == hawkconfig.ProviderXiaomiTokenPlan {
			hint = "Token Plan: enter pick region (cn/sgp/ams) then key · g change region"
		}
		if targetIdx >= 0 && targetIdx < len(rows) && rows[targetIdx].ID == hawkconfig.ProviderZAICoding {
			hint = "Coding Plan: enter pick region (international/cn) then key · g change region"
		}

		b.WriteString("\n" + mutedStyle.Render(indent+hint))
	} else {
		hints := "enter use gateway · k view key · delete remove · r refresh"
		if targetIdx >= 0 && targetIdx < len(rows) && rows[targetIdx].ID == hawkconfig.ProviderXiaomiTokenPlan {
			hints = "enter · g region · k key · delete · r refresh"
		}
		if targetIdx >= 0 && targetIdx < len(rows) && rows[targetIdx].ID == hawkconfig.ProviderZAICoding {
			hints = "enter · g region · k key · delete · r refresh"
		}

		b.WriteString("\n" + configTableSelectionFooter(len(rows), m.configScroll, end, mutedStyle, hints))
	}
	return m.configTabShellView(b.String())
}

func (m chatModel) selectedConfigGateway() (configGatewayRow, bool) {
	rows := m.configGatewayRows()
	if m.configSel < 0 || m.configSel >= len(rows) {
		return configGatewayRow{}, false
	}
	return rows[m.configSel], true
}

func (m chatModel) handleConfigGatewaysDelete() chatModel {
	if row, ok := m.selectedConfigGateway(); ok && row.HasKey {
		return m.beginConfigGatewayKeyRemove(row.ID)
	}
	return m
}

func (m chatModel) handleConfigGatewaysEsc() chatModel {
	if m.configKeysPendingRemove != "" {
		return m.clearConfigGatewayKeyRemove()
	}
	return m.closeConfigPanel()
}

func (m chatModel) handleConfigGatewaysSelect() (chatModel, tea.Cmd) {
	if m.configKeysPendingRemove != "" {
		return m.advanceConfigGatewayKeyRemove()
	}
	rows := m.configGatewayRows()
	refreshIdx := len(rows)
	if m.configSel == refreshIdx {
		return m.refreshConfigGateway()
	}
	if m.configSel < 0 || m.configSel >= len(rows) {
		return m, nil
	}
	row := rows[m.configSel]
	if row.ID == hawkconfig.ProviderXiaomiTokenPlan {
		if !row.HasKey || row.RegionRequired {
			m.configGatewayFocus = m.configSel
			return m.startConfigXiaomiTokenPlanRegion(), nil
		}
	}
	if row.ID == hawkconfig.ProviderZAICoding && (!row.HasKey || row.RegionRequired) {
		m.configGatewayFocus = m.configSel
		return m.startConfigZAIRegion(row.ID), nil
	}

	if !row.HasKey {
		if row.ID == configProviderOllama {
			return m.startConfigOllamaURL()
		}
		m.configGatewayFocus = m.configSel
		return m.startConfigKeyForProvider(row.ID)
	}
	gw := row.ID
	m.configGatewayFocus = m.configSel
	m.configModelProvider = gw
	_ = hawkconfig.SetGlobalSetting("provider", gw)
	if active := hawkconfig.ActiveProvider(context.Background()); active != "" {
		m.session.SetProvider(active)
	}
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
	m = m.refreshConfigGatewayRows()
	if msg.err != nil {
		m.configNotice = sanitizeConfigNotice(hawkconfig.FormatConfigProviderError(msg.providerID, msg.err))
		return m
	}
	m.configNotice = msg.summary
	if m.configTab == configTabModels && strings.TrimSpace(m.configModelProvider) == msg.providerID {
		m.configModelOptions = loadConfigModelOptions(msg.providerID)
	}
	return m
}

func (m chatModel) focusConfigActiveGateway() chatModel {
	rows := m.configGatewayRows()
	windowSize := m.configVisibleRows()
	if i := m.activeGatewayRowIndex(rows); i >= 0 {
		m.configGatewayFocus = i
		m.configSel = i
		if m.configSel < m.configScroll {
			m.configScroll = m.configSel
		}
		if m.configSel >= m.configScroll+windowSize {
			m.configScroll = m.configSel - windowSize + 1
		}
	}
	return m
}

func (m chatModel) trackConfigGatewayFocus() chatModel {
	if m.configTab != configTabGateways {
		return m
	}
	rows := len(m.configGatewayRows())
	if m.configSel >= 0 && m.configSel < rows {
		m.configGatewayFocus = m.configSel
	}
	return m
}
