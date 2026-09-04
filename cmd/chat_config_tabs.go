package cmd

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func (m chatModel) configStatus() (gateway, model string, configured bool) {
	ctx := context.Background()
	if !graycodeconfig.HasConfiguredDeploymentCached(ctx) {
		return "none", "", false
	}
	gw := strings.TrimSpace(m.configModelProvider)
	if gw != "" && graycodeconfig.IsSetupGateway(gw) {
		gw = graycodeconfig.GatewayDisplayName(gw)
	} else if active := graycodeconfig.ActiveGateway(ctx); active != "" {
		gw = graycodeconfig.GatewayDisplayName(active)
	} else {
		gw = "none"
	}
	if m.session != nil {
		model = strings.TrimSpace(m.session.Model())
	}
	if model == "" {
		model = strings.TrimSpace(graycodeconfig.ActiveModel(ctx))
	}
	return gw, model, true
}

const configTabGap = 4

func configTabDot(active bool) string {
	if active {
		return icons.CircleFilled()
	}
	return icons.CircleOutline()
}

func configTabLabelStyle(active bool) lipgloss.Style {
	if active {
		return lipgloss.NewStyle().Foreground(graycodeColor).Bold(true)
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
		dot := configTabDot(i == active)
		parts = append(parts, configTabLabelStyle(i == active).Render(dot+" "+label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m chatModel) configTabShellView(body string) string {
	var b strings.Builder
	b.WriteString(configTitleStyle().Render(icons.Cog()+" Setup") + "\n")
	b.WriteString(renderConfigStatusLine(m) + "\n\n")
	b.WriteString(renderConfigTabBar(m.configTab) + "\n")
	dividerWidth := m.width - 2
	if dividerWidth < 52 {
		dividerWidth = 52
	}
	b.WriteString(configMutedStyle().Render(strings.Repeat("─", dividerWidth)) + "\n\n")
	if notice := renderConfigNotice(m.configNoticeForView()); notice != "" {
		b.WriteString(notice + "\n\n")
	}
	b.WriteString(body)
	b.WriteString("\n" + m.configHelpLine())
	return b.String()
}

func (m chatModel) switchConfigTab(tab int) (chatModel, tea.Cmd) {
	if tab < configTabGateways || tab > configTabModels {
		return m, nil
	}
	if m.configTab == configTabModels && tab != configTabModels {
		m = m.stopConfigModelSearch(true)
	}
	ctx := context.Background()
	if tab == configTabModels && !graycodeconfig.HasConfiguredDeploymentCached(ctx) {
		tab = configTabGateways
		m.configNotice = "Select a gateway first · enter · paste API key"
		m = m.focusConfigActiveGateway()
	}
	m.configTab = tab
	m.configSel = 0
	m.configScroll = 0
	m.configKeysPendingRemove = ""
	m.configKeysRemoveStep = 0
	m.configNotice = ""
	if tab == configTabModels {
		if strings.TrimSpace(m.configModelProvider) == "" {
			m.configModelProvider = firstRunModelProvider(m)
		}
		return m.beginConfigModelsTab()
	}
	if tab == configTabGateways {
		m = m.refreshConfigGatewayRows()
		m = m.focusConfigActiveGateway()
	}
	return m, nil
}

func (m chatModel) openConfigAtTab(tab int) (chatModel, tea.Cmd) {
	ctx := context.Background()
	m.configOpen = true
	m.configEntry = configEntryNone
	m.configSaving = false
	m.configSel = 0
	m.configScroll = 0
	m.viewDirty = true
	graycodeconfig.RefreshConfigCredSnapshot(ctx)
	m = m.refreshConfigGatewayRows()
	setup := graycodeconfig.EvaluateSetupCached(ctx)

	if tab < 0 {
		if setup.HasCredentials {
			tab = configTabModels
		} else {
			tab = configTabGateways
		}
	}
	m.configTab = tab
	if tab == configTabModels {
		m.configModelProvider = firstRunModelProvider(m)
		if strings.TrimSpace(m.configModelProvider) == "" {
			m.configTab = configTabGateways
			m = m.focusConfigActiveGateway()
			m.configNotice = "Select a gateway · press enter · paste your API key"
			return m, nil
		}
		m.configNotice = ""
		return m.beginConfigModelsTab()
	}
	if tab == configTabGateways {
		m = m.focusConfigActiveGateway()
		if !setup.HasCredentials {
			m.configNotice = "Select a gateway · press enter · paste your API key"
		}
	}
	return m, nil
}
