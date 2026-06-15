package cmd

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

var zaiRegions = []struct {
	id    string
	label string
}{
	{id: "global", label: "Global (api.z.ai) — recommended for most users"},
	{id: "cn", label: "China (open.bigmodel.cn)"},
}

func zaiRegionIndex(region string) int {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		return 0
	}
	for i, r := range zaiRegions {
		if r.id == region {
			return i
		}
	}
	return 0
}

func (m chatModel) startConfigZAIRegion(providerID string) chatModel {
	m.configEntry = configEntryZAIRegion
	m.configProvider = providerID
	if hawkconfig.NeedsZAIRegion(providerID) {
		m.configZAIRegionSel = 0
	} else {
		m.configZAIRegionSel = zaiRegionIndex(hawkconfig.ZAIRegionLabel(providerID))
	}
	name := hawkconfig.GatewayDisplayName(providerID)
	notice := "Select " + name + " region (↑↓ · enter · esc cancel)"
	if saved := hawkconfig.ZAIRegionLabel(providerID); saved != "" {
		notice = name + " region · current " + saved + " (↑↓ · enter · esc cancel)"
	}
	m.configNotice = notice
	return m
}

func (m chatModel) configZAIRegionView() string {
	mutedStyle := configMutedStyle()
	accentStyle := configAccentStyle()
	rowStyle := configRowStyle()
	var b strings.Builder
	prov := m.configProvider
	name := hawkconfig.GatewayDisplayName(prov)
	b.WriteString(renderConfigBreadcrumb(name+" region") + "\n\n")
	for i, r := range zaiRegions {
		prefix := "  "
		if i == m.configZAIRegionSel {
			prefix = "> "
		}
		line := prefix + r.label
		if i == m.configZAIRegionSel {
			b.WriteString(accentStyle.Render(line) + "\n")
		} else {
			b.WriteString(rowStyle.Render(line) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("  Coding Plan uses dedicated /coding/paas/v4 on the chosen region"))
	return m.configTabShellView(b.String())
}

func (m chatModel) handleConfigZAIRegionKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.configEntry = configEntryNone
		m.configProvider = ""
		if idx := m.configGatewayRowIndex(m.configProvider); idx >= 0 {
			m.configSel = idx
		}
		m.configNotice = ""
		return m, nil
	case tea.KeyUp:
		if m.configZAIRegionSel > 0 {
			m.configZAIRegionSel--
		}
		return m, nil
	case tea.KeyDown:
		if m.configZAIRegionSel < len(zaiRegions)-1 {
			m.configZAIRegionSel++
		}
		return m, nil
	case tea.KeyEnter:
		if m.configZAIRegionSel < 0 || m.configZAIRegionSel >= len(zaiRegions) {
			return m, nil
		}
		region := zaiRegions[m.configZAIRegionSel].id
		prov := m.configProvider
		if err := hawkconfig.SetZAIRegion(prov, region); err != nil {
			m.configNotice = "Region: " + err.Error()
			return m, nil
		}
		InvalidateModelCacheProvider(prov)
		m.configEntry = configEntryNone
		ctx := context.Background()
		if post := strings.TrimSpace(m.configPostSaveKeysProvider); post == prov {
			m.configPostSaveKeysProvider = ""
			return m.startConfigKeyReplace(post)
		}
		if hawkconfig.HasStoredCredentialForProvider(ctx, prov) {
			m.configNotice = "Region saved (" + region + ") — press r to refresh models"
			if idx := m.configGatewayRowIndex(prov); idx >= 0 {
				m.configSel = idx
			}
			return m, nil
		}
		m.configNotice = "Region saved (" + region + ") — paste Z.AI API key"
		return m.startConfigKeyForProvider(prov)
	default:
		return m, nil
	}
}
