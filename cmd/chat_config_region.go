package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func gatewayRegionOptionIndex(providerID, region string) int {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		return 0
	}
	opts := hawkconfig.GatewayRegionOptions(providerID)
	for i, r := range opts {
		if strings.EqualFold(r.Value, region) {
			return i
		}
	}
	return 0
}

func (m chatModel) closeConfigEntry() chatModel {
	m.configEntry = configEntryNone
	m.configProvider = ""
	return m
}

func (m chatModel) startConfigGatewayRegion(providerID string) chatModel {
	switch providerID {
	case hawkconfig.ProviderXiaomiTokenPlan:
		m.configEntry = configEntryXiaomiRegion
	case hawkconfig.ProviderZAICoding, hawkconfig.ProviderZAIPayg:
		m.configEntry = configEntryZAIRegion
	default:
		m.configEntry = configEntryGatewayRegion
	}
	m.configProvider = providerID
	idx := 0
	if !hawkconfig.NeedsGatewayRegion(providerID) {
		idx = gatewayRegionOptionIndex(providerID, hawkconfig.GatewayRegionLabel(providerID))
	}
	m.configGatewayRegionSel = idx
	m.configXiaomiRegionSel = idx
	m.configZAIRegionSel = idx
	name := hawkconfig.GatewayDisplayName(providerID)
	notice := fmt.Sprintf("Select %s region (↑↓ · enter · esc cancel)", name)
	if saved := hawkconfig.GatewayRegionLabel(providerID); saved != "" {
		notice = fmt.Sprintf("%s region · current %s (↑↓ · enter · esc cancel)", name, saved)
	}
	m.configNotice = notice
	return m
}

func (m chatModel) configGatewayRegionView() string {
	mutedStyle := configMutedStyle()
	accentStyle := configAccentStyle()
	rowStyle := configRowStyle()
	var b strings.Builder
	prov := m.configProvider
	name := hawkconfig.GatewayDisplayName(prov)
	b.WriteString(renderConfigBreadcrumb(name+" region") + "\n\n")
	opts := hawkconfig.GatewayRegionOptions(prov)
	for i, r := range opts {
		prefix := "  "
		if i == m.configGatewayRegionSel {
			prefix = "> "
		}
		label := r.DisplayName
		if label == "" {
			label = r.Value
		}
		if i == m.configGatewayRegionSel {
			b.WriteString(accentStyle.Render(prefix+label) + "\n")
		} else {
			b.WriteString(rowStyle.Render(prefix+label) + "\n")
		}
	}
	b.WriteString("\n" + mutedStyle.Render("  Press enter to apply region."))
	return m.configTabShellView(b.String())
}

func (m chatModel) handleConfigGatewayRegionKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	opts := hawkconfig.GatewayRegionOptions(m.configProvider)
	count := len(opts)
	if count == 0 {
		return m.closeConfigEntry(), nil
	}
	switch msg.String() {
	case "up", "k":
		if m.configGatewayRegionSel > 0 {
			m.configGatewayRegionSel--
			m.configXiaomiRegionSel = m.configGatewayRegionSel
			m.configZAIRegionSel = m.configGatewayRegionSel
		}
		return m, nil
	case "down", "j":
		if m.configGatewayRegionSel < count-1 {
			m.configGatewayRegionSel++
			m.configXiaomiRegionSel = m.configGatewayRegionSel
			m.configZAIRegionSel = m.configGatewayRegionSel
		}
		return m, nil
	case "enter":
		if m.configGatewayRegionSel >= 0 && m.configGatewayRegionSel < count {
			chosen := opts[m.configGatewayRegionSel]
			if err := hawkconfig.SetGatewayRegion(m.configProvider, chosen.Value); err != nil {
				m.configNotice = "Error saving region: " + err.Error()
				return m, nil
			}
			InvalidateModelCacheProvider(m.configProvider)
			m.configGatewayRowsDirty = true
			ctx := context.Background()
			if post := strings.TrimSpace(m.configPostSaveKeysProvider); post == m.configProvider {
				m.configPostSaveKeysProvider = ""
				return m.startConfigKeyReplace(m.configProvider)
			}
			if hawkconfig.HasStoredCredentialForProvider(ctx, m.configProvider) {
				m.configNotice = "Saved region for " + hawkconfig.GatewayDisplayName(m.configProvider)
				if idx := m.configGatewayRowIndex(m.configProvider); idx >= 0 {
					m.configSel = idx
				}
				return m.closeConfigEntry(), nil
			}
			return m.startConfigKeyForProvider(m.configProvider)
		}
		return m.closeConfigEntry(), nil
	case "esc":
		m.configPostSaveKeysProvider = ""
		return m.closeConfigEntry(), nil
	default:
		return m, nil
	}
}
