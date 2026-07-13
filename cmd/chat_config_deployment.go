package cmd

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

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
	return saveCredentialAsync(inference, secret)
}

func saveOllamaAsync(baseURL string) tea.Cmd {
	return func() tea.Msg {
		inference, err := hawkconfig.LocalCredentialInference(configProviderOllama)
		if err != nil {
			return configApplyCredentialsMsg{err: err}
		}
		inf := hawkconfig.CredentialInference{
			ProviderID:   inference.ProviderID,
			DeploymentID: inference.DeploymentID,
			EnvVar:       inference.EnvVar,
			DisplayName:  inference.DisplayName,
		}
		return saveCredentialAsync(inf, baseURL)()
	}
}

func saveCredentialAsync(inference hawkconfig.CredentialInference, secret string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if err := hawkconfig.SaveCredential(ctx, inference, secret); err != nil {
			return configApplyCredentialsMsg{
				err:          err,
				providerID:   inference.ProviderID,
				deploymentID: inference.DeploymentID,
			}
		}
		hawkconfig.InvalidateConfigUICache()
		hawkconfig.RefreshConfigCredSnapshot(ctx)
		result, err := hawkconfig.ApplyEyrieCredentialsForProvider(ctx, inference.ProviderID)
		if err != nil {
			return configApplyCredentialsMsg{
				err:          err,
				providerID:   inference.ProviderID,
				deploymentID: inference.DeploymentID,
			}
		}

		entries, listErr := hawkconfig.ListEngineModels(ctx, inference.ProviderID, false)
		if listErr != nil {
			return configApplyCredentialsMsg{
				err:          listErr,
				providerID:   inference.ProviderID,
				deploymentID: inference.DeploymentID,
			}
		}
		opts := configModelOptionsFromEyrie(entries)
		return configApplyCredentialsMsg{
			summary:      hawkconfig.FormatApplyCredentialsSummary(result),
			providerID:   inference.ProviderID,
			deploymentID: inference.DeploymentID,
			modelOptions: opts,
		}
	}
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
	selection := hawkconfig.EffectiveSelectionWithSettings(context.Background(), m.settings, hawkconfig.SelectionOptions{
		ProviderOverride: firstNonEmptyTrimmed(m.session.Provider(), m.settings.Provider),
		ModelOverride:    firstNonEmptyTrimmed(m.session.Model(), m.settings.Model),
	})
	if err := engine.RebuildSessionTransportForSettings(context.Background(), m.settings, m.session, selection, m.session.Provider()); err != nil {
		m.configNotice = sanitizeConfigNotice(err.Error())
	}
	syncSessionFromPersistedSelection(m.session)
	m.invalidateConnStatus()
	return m, nil
}
