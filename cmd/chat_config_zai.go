package cmd

import (
	tea "charm.land/bubbletea/v2"
)

func (m chatModel) configZAIRegionView() string {
	return m.configGatewayRegionView()
}

func (m chatModel) handleConfigZAIRegionKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	return m.handleConfigGatewayRegionKey(msg)
}
