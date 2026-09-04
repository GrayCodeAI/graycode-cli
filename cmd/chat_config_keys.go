package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

func credentialsStoreLabel() string {
	return graycodeconfig.CredentialStoreName()
}

func configGatewayRemovePrompt(step int, gatewayName string) string {
	switch step {
	case 2:
		return fmt.Sprintf("This permanently removes the key for %s — press enter again to confirm · esc cancel", gatewayName)
	default:
		return fmt.Sprintf("Remove key for %s? press enter to continue · esc cancel", gatewayName)
	}
}

func configGatewayRemoveNotice(step int, gatewayName string) string {
	switch step {
	case 2:
		return fmt.Sprintf("Press enter again to permanently remove the key for %s · esc cancel", gatewayName)
	default:
		return fmt.Sprintf("Remove key for %s? Press enter to continue · esc cancel", gatewayName)
	}
}

func (m chatModel) configKeyDetailView() string {
	mutedStyle := configMutedStyle()
	accentStyle := configAccentStyle()
	activeStyle := configActiveStyle()
	providerName := strings.TrimSpace(m.configProvider)
	displayName := graycodeconfig.GatewayDisplayName(providerName)
	masked := graycodeconfig.MaskCredentialForProvider(context.Background(), providerName)

	var b strings.Builder
	b.WriteString(renderConfigBreadcrumb(displayName+" key") + "\n\n")
	b.WriteString(mutedStyle.Render("  Gateway: ") + accentStyle.Render(displayName) + "\n")
	b.WriteString(mutedStyle.Render("  Key: ") + activeStyle.Render(masked) + "\n")
	b.WriteString(mutedStyle.Render("  Stored in: "+credentialsStoreLabel()) + "\n")
	if reg := graycodeconfig.GatewayRegionLabel(providerName); reg != "" || graycodeconfig.NeedsGatewayRegion(providerName) || graycodeconfig.HasRegionOptions(providerName) {
		if reg == "" {
			reg = "(not set — press g)"
		}
		b.WriteString(mutedStyle.Render("  Region: ") + accentStyle.Render(reg) + "\n")
	}
	return m.configTabShellView(b.String())
}

func (m chatModel) startConfigKeyView(provider string) chatModel {
	m.configEntry = configEntryKeyView
	m.configProvider = provider
	m.configKeysPendingRemove = ""
	m.configKeysRemoveStep = 0
	m.configNotice = ""
	return m
}

func (m chatModel) startConfigKeyForProvider(provider string) (chatModel, tea.Cmd) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return m, nil
	}
	if graycodeconfig.NeedsGatewayRegion(provider) {
		m.configPostSaveKeysProvider = provider
		return m.startConfigGatewayRegion(provider), nil
	}

	name := graycodeconfig.GatewayDisplayName(provider)
	m.configNotice = "Paste API key for " + name
	return m.startConfigEntry(configEntryAPIKeyPaste, provider)
}

func (m chatModel) startConfigKeyReplace(provider string) (chatModel, tea.Cmd) {
	if graycodeconfig.NeedsGatewayRegion(provider) {
		m.configPostSaveKeysProvider = provider
		return m.startConfigGatewayRegion(provider), nil
	}

	m.configReplaceProvider = provider
	m.configEntry = configEntryNone
	m.configNotice = "Paste replacement API key for " + graycodeconfig.GatewayDisplayName(provider)
	return m.startConfigEntry(configEntryAPIKeyPaste, provider)
}

func (m chatModel) beginConfigGatewayKeyRemove(provider string) chatModel {
	m.configKeysPendingRemove = provider
	m.configKeysRemoveStep = 1
	name := graycodeconfig.GatewayDisplayName(provider)
	m.configNotice = configGatewayRemoveNotice(1, name)
	return m
}

func (m chatModel) clearConfigGatewayKeyRemove() chatModel {
	m.configKeysPendingRemove = ""
	m.configKeysRemoveStep = 0
	if strings.Contains(m.configNotice, "Remove key for") ||
		strings.Contains(m.configNotice, "Press enter to continue") ||
		strings.Contains(m.configNotice, "permanently remove") {
		m.configNotice = ""
	}
	return m
}

func (m chatModel) advanceConfigGatewayKeyRemove() (chatModel, tea.Cmd) {
	trimmedProvider := strings.TrimSpace(m.configKeysPendingRemove)
	if trimmedProvider == "" {
		return m, nil
	}
	if m.configKeysRemoveStep < 2 {
		m.configKeysRemoveStep = 2
		name := graycodeconfig.GatewayDisplayName(trimmedProvider)
		m.configNotice = configGatewayRemoveNotice(2, name)
		return m, nil
	}
	return m.confirmConfigGatewayKeyRemove()
}

func (m chatModel) confirmConfigGatewayKeyRemove() (chatModel, tea.Cmd) {
	trimmedProvider := strings.TrimSpace(m.configKeysPendingRemove)
	if trimmedProvider == "" {
		return m, nil
	}
	m.configKeysPendingRemove = ""
	m.configKeysRemoveStep = 0
	m.configSaving = true
	m.configNotice = fmt.Sprintf("Removing key for %s…", graycodeconfig.GatewayDisplayName(trimmedProvider))
	if m.configEntry == configEntryKeyView {
		m.configEntry = configEntryNone
		m.configProvider = ""
	}
	return m, removeCredentialAsync(trimmedProvider)
}

func (m chatModel) handleConfigKeyViewKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	trimmedProvider := strings.TrimSpace(m.configProvider)
	switch key := msg.Key(); key.Code {
	case tea.KeyEsc:
		m.configEntry = configEntryNone
		m.configProvider = ""
		m = m.clearConfigGatewayKeyRemove()
		if idx := m.configGatewayRowIndex(trimmedProvider); idx >= 0 {
			m.configSel = idx
		}
		return m, nil
	case tea.KeyDelete, tea.KeyBackspace:
		if trimmedProvider == "" {
			return m, nil
		}
		return m.beginConfigGatewayKeyRemove(trimmedProvider), nil
	case tea.KeyEnter:
		if m.configKeysPendingRemove != "" {
			return m.advanceConfigGatewayKeyRemove()
		}
		if trimmedProvider == "" {
			return m, nil
		}
		return m.startConfigKeyReplace(trimmedProvider)
	default:
		if (graycodeconfig.HasRegionOptions(trimmedProvider) || graycodeconfig.GatewayRegionLabel(trimmedProvider) != "") && strings.EqualFold(key.Text, "g") {
			return m.startConfigGatewayRegion(trimmedProvider), nil
		}
		return m, nil
	}
}
