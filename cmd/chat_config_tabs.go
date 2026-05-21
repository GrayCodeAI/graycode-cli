package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func (m chatModel) configStatusLine() string {
	ctx := context.Background()
	if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		return "Gateway: none · no API key — add one in Keys"
	}
	gw := strings.TrimSpace(m.configModelProvider)
	if gw != "" && hawkconfig.IsSetupGateway(gw) {
		gw = hawkconfig.GatewayDisplayName(gw)
	} else if active := hawkconfig.ActiveGateway(ctx); active != "" {
		gw = hawkconfig.GatewayDisplayName(active)
	} else {
		gw = "none"
	}
	model := ""
	if m.session != nil {
		model = strings.TrimSpace(m.session.Model())
	}
	if model == "" {
		model = strings.TrimSpace(hawkconfig.ActiveModel(ctx))
	}
	if model == "" {
		return fmt.Sprintf("Gateway: %s · no model selected", gw)
	}
	return fmt.Sprintf("Gateway: %s · Model: %s", gw, model)
}

func renderConfigTabBar(active int, tabStyle, activeStyle lipgloss.Style) string {
	var parts []string
	for i, label := range configTabLabels {
		if i == active {
			parts = append(parts, activeStyle.Render(" "+label+" "))
		} else {
			parts = append(parts, tabStyle.Render(" "+label+" "))
		}
	}
	return strings.Join(parts, "  ")
}

func (m chatModel) configTabShellView(body string) string {
	var b strings.Builder
	b.WriteString(configTitleStyle().Render("⚙ Setup") + "\n")
	b.WriteString(configMutedStyle().Render(m.configStatusLine()) + "\n\n")
	tabStyle := configMutedStyle()
	activeTabStyle := configSelectedStyle()
	b.WriteString(renderConfigTabBar(m.configTab, tabStyle, activeTabStyle) + "\n")
	b.WriteString(configMutedStyle().Render(strings.Repeat("─", 52)) + "\n\n")
	if notice := renderConfigNotice(m.configNotice); notice != "" {
		b.WriteString(notice + "\n\n")
	}
	b.WriteString(body)
	b.WriteString("\n" + m.configHelpLine())
	return b.String()
}

func (m chatModel) switchConfigTab(tab int) (chatModel, tea.Cmd) {
	if tab < configTabKeys || tab > configTabModels {
		return m, nil
	}
	ctx := context.Background()
	if tab == configTabModels && !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		tab = configTabKeys
		m.configNotice = "Add an API key first — select Add API key, press enter, paste"
		m.configSel = m.configKeysAddRowIndex()
	}
	m.configTab = tab
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = ""
	if tab == configTabModels {
		if strings.TrimSpace(m.configModelProvider) == "" {
			m.configModelProvider = firstRunModelProvider(m)
		}
		return m.beginConfigModelsTab()
	}
	if tab == configTabGateways {
		m.configGatewayFocus = 0
	}
	return m, nil
}

func (m chatModel) openConfigAtTab(tab int) (chatModel, tea.Cmd) {
	ctx := context.Background()
	m.configOpen = true
	m.configMenu = configMenuNone
	m.configEntry = configEntryNone
	m.configSaving = false
	m.configSel = 0
	m.configScroll = 0
	m.viewDirty = true
	hawkconfig.RefreshConfigCredSnapshot(ctx)
	setup := hawkconfig.EvaluateSetupCached(ctx)

	if tab < 0 {
		if setup.HasCredentials {
			tab = configTabModels
		} else {
			tab = configTabKeys
		}
	}
	m.configTab = tab
	if tab == configTabModels {
		m.configModelProvider = firstRunModelProvider(m)
		m.configNotice = ""
		return m.beginConfigModelsTab()
	}
	if tab == configTabKeys && !setup.HasCredentials {
		m.configNotice = "Select Add API key · press enter · paste your key"
		m.configSel = 0
	}
	return m, nil
}
