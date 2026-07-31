package cmd

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

var zaiRegions = []struct {
	id    string
	label string
}{
	{id: "international", label: "International (api.z.ai)"},
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
	return m.startConfigGatewayRegion(providerID)
}

func (m chatModel) configZAIRegionView() string {
	return m.configGatewayRegionView()
}

func (m chatModel) handleConfigZAIRegionKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	return m.handleConfigGatewayRegionKey(msg)
}
