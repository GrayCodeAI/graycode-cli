package cmd

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
)

type configHubOption struct {
	action string
	label  string
}

func (m chatModel) configHubOptions() []configHubOption {
	var out []configHubOption
	if hawkconfig.EvaluateSetup(context.Background()).HasCredentials {
		out = append(out, configHubOption{action: "model", label: "Pick model"})
	}
	out = append(
		out,
		configHubOption{action: "apikey", label: "Paste API key"},
		configHubOption{action: "ollama", label: "Ollama (local — no key)"},
	)
	return out
}

func (m chatModel) configHubLabels() []string {
	opts := m.configHubOptions()
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = o.label
	}
	return out
}

func (m chatModel) configHubNotice() string {
	if m.configSaving {
		return "Working…"
	}
	st := hawkconfig.EvaluateSetup(context.Background())
	if !st.HasCredentials {
		return "Step 1: choose how to connect"
	}
	prov := strings.TrimSpace(m.session.Provider())
	model := strings.TrimSpace(m.session.Model())
	if prov == "" {
		prov = "unknown provider"
	}
	if model != "" {
		return fmt.Sprintf("Current: %s · %s", prov, model)
	}
	return fmt.Sprintf("Current: %s · pick a model to start", prov)
}

func (m chatModel) openConfigHub(firstRun bool) chatModel {
	m.configOpen = true
	m.configMenu = "hub"
	m.configSel = 0
	m.configScroll = 0
	m.configEntry = ""
	m.configSaving = false
	m.configGuideAfterKey = firstRun
	m.configNotice = m.configHubNotice()
	m.viewDirty = true
	return m
}

func (m chatModel) beginConfigModelPicker() (chatModel, tea.Cmd) {
	m.configMenu = "model"
	m.configSel = 0
	m.configScroll = 0
	m.configModelProvider = firstRunModelProvider(m)
	m.configModelOptions = loadConfigModelOptions(m.configModelProvider)
	if len(m.configModelOptions) == 0 {
		m.configNotice = "Loading models…"
		return m, fetchModelsAsync(m.configModelProvider)
	}
	m.configNotice = "Pick a model"
	return m, nil
}

func (m chatModel) returnToOllamaURLAfterError(err error) (chatModel, tea.Cmd) {
	m.configSaving = false
	url := strings.TrimSpace(m.configPendingOllamaURL)
	if url == "" {
		url = "http://localhost:11434/v1"
	}
	if err != nil {
		m.configNotice = hawkconfig.FormatConfigProviderError("ollama", err)
	}
	return m.startConfigOllamaURLWithValue(url)
}

func formatConfigApplyError(providerID string, err error) string {
	return eyrieclient.FormatSetupError(providerID, err)
}
