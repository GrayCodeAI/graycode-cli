package cmd

import (
	"context"
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
	if strings.TrimSpace(m.configModelProvider) == "" {
		m.configModelProvider = firstRunModelProvider(m)
	}
	m.configModelOptions = loadConfigModelOptions(m.configModelProvider)
	if len(m.configModelOptions) == 0 {
		m.configSaving = true
		m.configNotice = "Loading models…"
		return m, fetchModelsAsync(m.configModelProvider)
	}
	return m, nil
}

func (m chatModel) returnToOllamaURLAfterError(err error) (chatModel, tea.Cmd) {
	m.configSaving = false
	m.configTab = configTabKeys
	url := strings.TrimSpace(m.configPendingOllamaURL)
	if url == "" {
		url = configDefaultOllamaURL
	}
	if err != nil {
		m.configNotice = hawkconfig.FormatConfigProviderError(configProviderOllama, err)
	}
	return m.startConfigOllamaURLWithValue(url)
}

type configRefreshCatalogMsg struct {
	summary string
	err     error
}

func refreshCatalogAsync() tea.Cmd {
	return func() tea.Msg {
		summary, err := hawkconfig.RefreshModelCatalogV1(context.Background())
		return configRefreshCatalogMsg{summary: summary, err: err}
	}
}

func (m chatModel) handleConfigRefreshCatalogMsg(msg configRefreshCatalogMsg) chatModel {
	m.configSaving = false
	InvalidateModelCache()
	if msg.err != nil {
		m.configNotice = sanitizeConfigNotice(msg.err.Error())
		return m
	}
	m.configNotice = strings.TrimSpace(strings.Split(msg.summary, "\n")[0])
	if m.configNotice == "" {
		m.configNotice = "Model catalog refreshed"
	}
	return m
}
