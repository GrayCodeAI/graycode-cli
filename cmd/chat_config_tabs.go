package cmd

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func (m chatModel) configStatus() (gateway, model string, configured bool) {
	ctx := context.Background()
	if !hawkconfig.HasConfiguredDeploymentCached(ctx) {
		return "none", "", false
	}
	gw := strings.TrimSpace(m.configModelProvider)
	if gw != "" && hawkconfig.IsSetupGateway(gw) {
		gw = hawkconfig.GatewayDisplayName(gw)
	} else if active := hawkconfig.ActiveGateway(ctx); active != "" {
		gw = hawkconfig.GatewayDisplayName(active)
	} else {
		gw = "none"
	}
	if m.session != nil {
		model = strings.TrimSpace(m.session.Model())
	}
	if model == "" {
		model = strings.TrimSpace(hawkconfig.ActiveModel(ctx))
	}
	return gw, model, true
}

const (
	configTabDotFilled = "●"
	configTabDotEmpty  = "○"
	configTabGap       = 4
)

func configTabLabelStyle(active bool) lipgloss.Style {
	if active {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	}
	return configMutedStyle()
}

func renderConfigTabBar(active int) string {
	if active < 0 || active >= len(configTabLabels) {
		active = 0
	}
	parts := make([]string, 0, len(configTabLabels)*2)
	for i, label := range configTabLabels {
		if i > 0 {
			parts = append(parts, strings.Repeat(" ", configTabGap))
		}
		dot := configTabDotEmpty
		if i == active {
			dot = configTabDotFilled
		}
		parts = append(parts, configTabLabelStyle(i == active).Render(dot+" "+label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m chatModel) configTabShellView(body string) string {
	var b strings.Builder
	b.WriteString(configTitleStyle().Render("⚙ Setup") + "\n")
	b.WriteString(renderConfigStatusLine(m) + "\n\n")
	b.WriteString(renderConfigTabBar(m.configTab) + "\n")
	dividerWidth := m.width - 2
	if dividerWidth < 52 {
		dividerWidth = 52
	}
	b.WriteString(configMutedStyle().Render(strings.Repeat("─", dividerWidth)) + "\n\n")
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
	if m.configTab == configTabModels && tab != configTabModels {
		m = m.stopConfigModelSearch(true)
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
