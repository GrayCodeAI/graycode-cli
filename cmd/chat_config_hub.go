package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/GrayCodeAI/eyrie/catalog/registry"
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
	} else if providerHasLiveFetcher(m.configModelProvider) && catalogPricesAreStale(m.configModelOptions) {
		// Cached catalog has entries but prices are stale (all zero). Refresh
		// in the background so live fetcher prices replace them.
		m.configSaving = true
		m.configNotice = "Refreshing prices…"
	}
	if m.configSaving {
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

// catalogPricesAreStale returns true when every model entry has zero pricing
// despite having context metadata — the compiled catalog was cached before
// live fetcher pricing was available.
func catalogPricesAreStale(opts []configModelOption) bool {
	if len(opts) == 0 {
		return false
	}
	for _, o := range opts {
		if o.InputPricePer1M > 0 || o.OutputPricePer1M > 0 {
			return false // at least one entry has real pricing
		}
	}
	// All entries are zero-priced — likely stale if a live fetcher exists.
	return true
}

func providerHasLiveFetcher(providerID string) bool {
	for _, key := range registry.LiveFetcherKeys() {
		if key == providerID {
			return true
		}
	}
	return false
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
