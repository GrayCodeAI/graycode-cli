package cmd

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

type configRemoveCredentialMsg struct {
	provider string
	removed  []string
	err      error
}

func removeCredentialAsync(provider string) tea.Cmd {
	return func() tea.Msg {
		removed, err := graycodeconfig.RemoveStoredCredential(context.Background(), provider)
		return configRemoveCredentialMsg{
			provider: provider,
			removed:  removed,
			err:      err,
		}
	}
}

func (m chatModel) handleConfigRemoveCredentialMsg(msg configRemoveCredentialMsg) (chatModel, tea.Cmd) {
	m.configSaving = false
	if msg.err != nil {
		m.configNotice = sanitizeConfigNotice(msg.err.Error())
		return m, nil
	}
	delete(modelCache, msg.provider)
	ctx := context.Background()
	graycodeconfig.RefreshConfigCredSnapshot(ctx)
	m = m.refreshConfigGatewayRows()
	if graycodeconfig.ShouldClearSelectionAfterCredentialRemove(ctx, msg.provider) {
		_ = graycodeconfig.ClearActiveSelection(ctx)
		m.configModelProvider = ""
		m.configModelOptions = nil
		m.session.SetProvider("")
		m.session.SetModel("")
	}
	m.configTab = configTabGateways
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = fmt.Sprintf("Removed API key for %s", graycodeconfig.GatewayDisplayName(msg.provider))
	if !graycodeconfig.HasConfiguredDeploymentCached(ctx) {
		m.configNotice += " — add an API key to continue"
	}
	next, cmd := m.rebuildSessionTransport()
	next.refreshWelcomeStatusSnapshot()
	next.rebuildWelcomeCache(next.blinkClosed)
	next.invalidateConnStatus()
	return next, cmd
}

func (m chatModel) openConfigRemoveKeyPanel() (chatModel, tea.Cmd) {
	next, cmd := m.openConfigAtTab(configTabGateways)
	if len(graycodeconfig.ConfiguredCredentialProviders()) == 0 {
		next.configNotice = "No stored API keys"
	}
	return next, cmd
}
