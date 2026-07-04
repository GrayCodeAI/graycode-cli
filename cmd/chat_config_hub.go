package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func (m chatModel) openConfigPanel() (chatModel, tea.Cmd) {
	return m.openConfigAtTab(-1)
}

func (m chatModel) beginConfigModelsTab() (chatModel, tea.Cmd) {
	m.configTab = configTabModels
	m.configSel = 0
	m.configScroll = 0
	m.configModelSearch = ""
	m.configModelSearchActive = false
	m.useConfigInput = false
	if strings.TrimSpace(m.configModelProvider) == "" {
		m.configModelProvider = firstRunModelProvider(m)
	}
	if strings.TrimSpace(m.configModelProvider) == "" {
		m.configTab = configTabGateways
		m.configNotice = "Select a gateway · press enter · paste your API key"
		return m.focusConfigActiveGateway(), nil
	}
	m.configModelOptions = loadConfigModelOptions(m.configModelProvider)
	if len(m.configModelOptions) == 0 {
		m.configSaving = true
		m.configNotice = "Loading models…"
		var cmds []tea.Cmd
		cmds = append(cmds, fetchModelsAsync(m.configModelProvider))
		if isXiaomiMimoProvider(m.configModelProvider) {
			cmds = append(cmds, fetchPlatformContextIndexCmd())
		}
		return m, tea.Batch(cmds...)
	}
	m = m.focusConfigActiveModelSelection()
	return m, nil
}

func (m chatModel) returnToOllamaURLAfterError(err error) (chatModel, tea.Cmd) {
	m.configSaving = false
	m.configTab = configTabGateways
	url := strings.TrimSpace(m.configPendingOllamaURL)
	if url == "" {
		url = configDefaultOllamaURL
	}
	if err != nil {
		m.configNotice = hawkconfig.FormatConfigProviderError(configProviderOllama, err)
	}
	return m.startConfigOllamaURLWithValue(url)
}
