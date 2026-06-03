package cmd

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

var xiaomiTokenPlanRegions = []struct {
	id    string
	label string
}{
	{id: "cn", label: "China (token-plan-cn)"},
	{id: "sgp", label: "Singapore (token-plan-sgp)"},
	{id: "ams", label: "Europe (token-plan-ams)"},
}

func xiaomiTokenPlanRegionIndex(region string) int {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		return 0
	}
	for i, r := range xiaomiTokenPlanRegions {
		if r.id == region {
			return i
		}
	}
	return 0
}

func (m chatModel) startConfigXiaomiTokenPlanRegion() chatModel {
	m.configEntry = configEntryXiaomiRegion
	m.configProvider = hawkconfig.ProviderXiaomiTokenPlan
	if hawkconfig.NeedsXiaomiTokenPlanRegion(hawkconfig.ProviderXiaomiTokenPlan) {
		m.configXiaomiRegionSel = 0
	} else {
		m.configXiaomiRegionSel = xiaomiTokenPlanRegionIndex(hawkconfig.XiaomiTokenPlanRegionLabel())
	}
	notice := "Select Token Plan region (↑↓ · enter · esc cancel)"
	if saved := hawkconfig.XiaomiTokenPlanRegionLabel(); saved != "" {
		notice = "Token Plan region · current " + saved + " (↑↓ · enter · esc cancel)"
	}
	m.configNotice = notice
	return m
}

func (m chatModel) configXiaomiRegionView() string {
	mutedStyle := configMutedStyle()
	accentStyle := configAccentStyle()
	rowStyle := configRowStyle()
	var b strings.Builder
	b.WriteString(renderConfigBreadcrumb("Xiaomi Token Plan region") + "\n\n")
	for i, r := range xiaomiTokenPlanRegions {
		prefix := "  "
		if i == m.configXiaomiRegionSel {
			prefix = "> "
		}
		line := prefix + r.label
		if i == m.configXiaomiRegionSel {
			b.WriteString(accentStyle.Render(line) + "\n")
		} else {
			b.WriteString(rowStyle.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("  Keys from plan-manage · tp- keys only on this gateway"))
	return m.configTabShellView(b.String())
}

func (m chatModel) handleConfigXiaomiRegionKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.configEntry = configEntryNone
		m.configProvider = ""
		if idx := m.configGatewayRowIndex(hawkconfig.ProviderXiaomiTokenPlan); idx >= 0 {
			m.configSel = idx
		}
		m.configNotice = ""
		return m, nil
	case tea.KeyUp:
		if m.configXiaomiRegionSel > 0 {
			m.configXiaomiRegionSel--
		}
		return m, nil
	case tea.KeyDown:
		if m.configXiaomiRegionSel < len(xiaomiTokenPlanRegions)-1 {
			m.configXiaomiRegionSel++
		}
		return m, nil
	case tea.KeyEnter:
		if m.configXiaomiRegionSel < 0 || m.configXiaomiRegionSel >= len(xiaomiTokenPlanRegions) {
			return m, nil
		}
		region := xiaomiTokenPlanRegions[m.configXiaomiRegionSel].id
		if err := hawkconfig.SetXiaomiTokenPlanRegion(region); err != nil {
			m.configNotice = "Region: " + err.Error()
			return m, nil
		}
		InvalidateModelCacheProvider(hawkconfig.ProviderXiaomiTokenPlan)
		m.configEntry = configEntryNone
		ctx := context.Background()
		if post := strings.TrimSpace(m.configPostSaveKeysProvider); post == hawkconfig.ProviderXiaomiTokenPlan {
			m.configPostSaveKeysProvider = ""
			return m.startConfigKeyReplace(post)
		}
		if hawkconfig.HasStoredCredentialForProvider(ctx, hawkconfig.ProviderXiaomiTokenPlan) {
			m.configNotice = "Region saved (" + region + ") — press r to refresh models"
			if idx := m.configGatewayRowIndex(hawkconfig.ProviderXiaomiTokenPlan); idx >= 0 {
				m.configSel = idx
			}
			return m, nil
		}
		m.configNotice = "Region saved (" + region + ") — paste Token Plan API key"
		return m.startConfigKeyForProvider(hawkconfig.ProviderXiaomiTokenPlan)
	default:
		return m, nil
	}
}