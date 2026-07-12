package cmd

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/GrayCodeAI/eyrie/runtime"
	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

type configApplyCredentialsMsg struct {
	summary      string
	err          error
	providerID   string
	deploymentID string
	modelOptions []configModelOption
}

func firstRunModelProvider(m chatModel) string {
	ctx := context.Background()
	if p := strings.TrimSpace(m.configModelProvider); p != "" && hawkconfig.HasStoredCredentialForProvider(ctx, p) {
		return p
	}
	if m.session != nil {
		if p := strings.TrimSpace(m.session.Provider()); p != "" && hawkconfig.HasStoredCredentialForProvider(ctx, p) {
			return p
		}
	}
	if p := strings.TrimSpace(hawkconfig.ActiveGateway(ctx)); p != "" && hawkconfig.HasStoredCredentialForProvider(ctx, p) {
		return p
	}
	if p := hawkconfig.DefaultModelProviderFilter(ctx); p != "" && hawkconfig.HasStoredCredentialForProvider(ctx, p) {
		return p
	}
	for _, p := range hawkconfig.AllSetupGateways() {
		if hawkconfig.HasStoredCredentialForProvider(ctx, p) {
			return p
		}
	}
	return ""
}

func saveProviderKeyAsync(inference hawkconfig.CredentialInference, secret string) tea.Cmd {
	return saveCredentialAsync(inference.ProviderID, secret)
}

func saveOllamaAsync(baseURL string) tea.Cmd {
	return saveCredentialAsync(configProviderOllama, baseURL)
}

func saveCredentialAsync(providerID, secret string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		result, err := hawkconfig.ConfigureProvider(ctx, providerID, secret)
		if err != nil {
			return configApplyCredentialsMsg{
				err:        err,
				providerID: providerID,
			}
		}

		return configApplyCredentialsMsg{
			summary: result.Summary, providerID: result.ProviderID,
			deploymentID: result.DeploymentID, modelOptions: toConfigModelOptionsFromHawk(result.Models),
		}
	}
}

func toConfigModelOptionsFromHawk(in []hawkconfig.ModelOption) []configModelOption {
	out := make([]configModelOption, len(in))
	for i, o := range in {
		out[i] = configModelOption{
			ID: o.ID, DisplayName: o.DisplayName, Owner: o.Owner,
			ContextWindow:   o.ContextWindow,
			InputPricePer1M: o.InputPricePer1M, OutputPricePer1M: o.OutputPricePer1M,
			PriceKnown: modelOptionPriceKnown(o.ID, o.DisplayName, o.InputPricePer1M, o.OutputPricePer1M, o.ContextWindow),
		}
	}
	return out
}

func (m chatModel) startConfigOllamaURL() (chatModel, tea.Cmd) {
	return m.startConfigOllamaURLWithValue(configDefaultOllamaURL)
}

func (m chatModel) startConfigOllamaURLWithValue(url string) (chatModel, tea.Cmd) {
	m.configEntry = configEntryOllamaURL
	m.configProvider = configProviderOllama
	if strings.TrimSpace(m.configNotice) == "" || strings.TrimSpace(m.configNotice) == "Working…" {
		m.configNotice = "Confirm Ollama URL (run: ollama serve)"
	}
	return m.startConfigURLInput(url)
}

func (m chatModel) startConfigURLInput(defaultURL string) (chatModel, tea.Cmd) {
	m.useConfigInput = true
	m.configInput.Reset()
	m.configInput.SetValue(defaultURL)
	m.configInput.Prompt = " url " + icons.ChevronRight() + " "
	m.configInput.Placeholder = defaultURL
	m.configInput.EchoMode = textinput.EchoNormal
	m.configInput.SetStyles(textinput.Styles{
		Focused: textinput.StyleState{
			Prompt: lipgloss.NewStyle().Foreground(hawkColor).Bold(true),
			Text:   lipgloss.NewStyle().Foreground(textPrimary),
		},
		Blurred: textinput.StyleState{
			Prompt: lipgloss.NewStyle().Foreground(hawkColor).Bold(true),
			Text:   lipgloss.NewStyle().Foreground(textPrimary),
		},
		Cursor: textinput.CursorStyle{
			Color: hawkColor,
		},
	})
	m.configInput.Focus()
	return m, textinput.Blink
}

func (m chatModel) handleConfigApplyCredentialsMsg(msg configApplyCredentialsMsg) (chatModel, tea.Cmd) {
	m.configSaving = false
	ctx := context.Background()
	hawkconfig.RefreshConfigCredSnapshot(ctx)
	if msg.err != nil {
		m.invalidateConnStatus()
		if msg.providerID == configProviderOllama {
			return m.returnToOllamaURLAfterError(msg.err)
		}
		notice := sanitizeConfigNotice(hawkconfig.FormatConfigProviderError(msg.providerID, msg.err))
		saved := hawkconfig.HasStoredCredentialForProvider(ctx, msg.providerID) ||
			strings.Contains(strings.ToLower(msg.err.Error()), "key saved in keychain")
		if saved {
			notice = "Key saved in " + credentialsStoreLabel() + " — provider rejected this key: " + notice
			if !strings.Contains(strings.ToLower(notice), "refresh") {
				notice += " · press r on " + hawkconfig.GatewayDisplayName(msg.providerID) + " to retry"
			}
		} else {
			notice = "Could not save key — " + notice
		}
		m.configNotice = notice
		m.configTab = configTabGateways
		if pid := strings.TrimSpace(msg.providerID); pid != "" {
			if idx := m.configGatewayRowIndex(pid); idx >= 0 {
				m.configSel = idx
			}
		}
		return m, nil
	}
	m.configPendingOllamaURL = ""
	m.configNotice = msg.summary
	InvalidateModelCache()
	m.configModelProvider = msg.providerID
	if len(msg.modelOptions) > 0 {
		modelCacheMu.Lock()
		modelCache[msg.providerID] = msg.modelOptions
		modelCacheMu.Unlock()
	}
	next, cmd := m.rebuildSessionTransport()
	next.refreshWelcomeStatusSnapshot()
	next.rebuildWelcomeCache(next.blinkClosed)
	next.invalidateConnStatus()
	if post := strings.TrimSpace(m.configPostSaveKeysProvider); post != "" {
		next.configPostSaveKeysProvider = ""
		next.configTab = configTabGateways
		next.configEntry = configEntryNone
		if idx := next.configGatewayRowIndex(post); idx >= 0 {
			next.configSel = idx
		}
		next.configNotice = "Key updated for " + hawkconfig.GatewayDisplayName(post)
		return next, cmd
	}
	if msg.providerID == configProviderOllama {
		_ = hawkconfig.SetGlobalSetting("provider", configProviderOllama)
		next.syncSessionSelection()
	}
	next.configGuideAfterKey = false
	if len(msg.modelOptions) == 0 {
		if msg.providerID == configProviderOllama {
			return next.returnToOllamaURLAfterError(fmt.Errorf("no models installed — run: ollama pull llama3.2"))
		}
		next.configTab = configTabGateways
		next.configNotice = "No models in catalog for " + msg.providerID + " — try another gateway"
		return next, cmd
	}
	next.configTab = configTabModels
	next.configSel = 0
	next.configScroll = 0
	next.configModelOptions = msg.modelOptions
	return next, cmd
}

func (m chatModel) rebuildSessionTransport() (chatModel, tea.Cmd) {
	selection := runtime.EffectiveSelection(context.Background(), runtime.SelectionOpts{
		ProviderOverride: firstNonEmptyTrimmed(m.session.Provider(), m.settings.Provider),
		ModelOverride:    firstNonEmptyTrimmed(m.session.Model(), m.settings.Model),
	})
	if err := engine.RebuildSessionTransport(context.Background(), m.session, selection, m.session.Provider()); err != nil {
		m.configNotice = sanitizeConfigNotice(err.Error())
	}
	syncSessionFromPersistedSelection(m.session)
	m.invalidateConnStatus()
	return m, nil
}
