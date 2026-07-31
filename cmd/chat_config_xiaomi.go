package cmd

import (
	"strings"

	tea "charm.land/bubbletea/v2"

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
	return m.startConfigGatewayRegion(hawkconfig.ProviderXiaomiTokenPlan)
}

func (m chatModel) configXiaomiRegionView() string {
	return m.configGatewayRegionView()
}

func (m chatModel) handleConfigXiaomiRegionKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	return m.handleConfigGatewayRegionKey(msg)
}
