package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/eyrieclient"
)

type configApplyCredentialsMsg struct {
	summary      string
	err          error
	providerID   string
	deploymentID string
	modelOptions []configModelOption
}

type configKeyResolvedMsg struct {
	secret string
	result hawkconfig.CredentialResolveResult
}

func firstRunModelProvider(m chatModel) string {
	ctx := context.Background()
	if p := hawkconfig.DefaultModelProviderFilter(ctx); p != "" {
		return p
	}
	return strings.TrimSpace(m.session.Provider())
}

func resolveKeyAsync(secret string) tea.Cmd {
	return func() tea.Msg {
		res := eyrieclient.ResolveCredentialForHost(context.Background(), secret)
		return configKeyResolvedMsg{
			secret: secret,
			result: credentialResolveFromRuntime(res),
		}
	}
}

func credentialResolveFromRuntime(res eyrieclient.CredentialResolveResult) hawkconfig.CredentialResolveResult {
	out := hawkconfig.CredentialResolveResult{
		FormatOK:    res.FormatOK,
		FormatError: res.FormatError,
		Providers:   make([]hawkconfig.CredentialProviderOption, len(res.Providers)),
	}
	for i, p := range res.Providers {
		out.Providers[i] = hawkconfig.CredentialProviderOption{
			ProviderID:   p.ProviderID,
			DeploymentID: p.DeploymentID,
			EnvVar:       p.EnvVar,
			DisplayName:  p.DisplayName,
			Inferred:     p.Inferred,
			RequiresKey:  p.RequiresKey,
			Rank:         p.Rank,
		}
	}
	return out
}

func credentialOptionFromHawk(in hawkconfig.CredentialInference) eyrieclient.CredentialProviderOption {
	return eyrieclient.CredentialProviderOption{
		ProviderID:   in.ProviderID,
		DeploymentID: in.DeploymentID,
		EnvVar:       in.EnvVar,
		DisplayName:  in.DisplayName,
	}
}

func saveProviderKeyAsync(inference hawkconfig.CredentialInference, secret string) tea.Cmd {
	return saveCredentialAsync(inference, secret)
}

func saveOllamaAsync(baseURL string) tea.Cmd {
	return func() tea.Msg {
		inference, err := eyrieclient.LocalCredentialInference(configProviderOllama)
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
		rtInf := eyrieclient.InferenceFromOption(credentialOptionFromHawk(inference))
		if err := eyrieclient.SaveCredentialForHost(ctx, rtInf, secret); err != nil {
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

		entries, listErr := eyrieclient.ListModelsForProvider(ctx, inference.ProviderID)
		if listErr != nil {
			return configApplyCredentialsMsg{
				err:          listErr,
				providerID:   inference.ProviderID,
				deploymentID: inference.DeploymentID,
			}
		}
		opts := configModelOptionsFromEyrie(entries)
		if len(opts) == 0 && result.Setup != nil {
			fallback := hawkconfig.OptionsFromSetupUI(result.Setup, inference.ProviderID)
			opts = toConfigModelOptionsFromHawk(fallback)
		}

		return configApplyCredentialsMsg{
			summary:      hawkconfig.FormatApplyCredentialsSummary(result),
			providerID:   inference.ProviderID,
			deploymentID: inference.DeploymentID,
			modelOptions: opts,
		}
	}
}

func toConfigModelOptionsFromHawk(in []hawkconfig.ModelOption) []configModelOption {
	out := make([]configModelOption, len(in))
	for i, o := range in {
		out[i] = configModelOption{
			ID:          o.ID,
			DisplayName: o.DisplayName,
		}
	}
	return out
}

func (m chatModel) configProvidersView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	opts := m.configProviderLabels()
	total := len(opts)

	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+configWindowSize {
		m.configScroll = m.configSel - configWindowSize + 1
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("🔑 Select gateway") + "\n\n")
	if notice := strings.TrimSpace(m.configNotice); notice != "" {
		b.WriteString(mutedStyle.Render(sanitizeConfigNotice(notice)) + "\n\n")
	}
	if m.configScroll > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more above ···", m.configScroll)) + "\n")
	}
	end := m.configScroll + configWindowSize
	if end > total {
		end = total
	}
	for i := m.configScroll; i < end; i++ {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		b.WriteString(lineStyle.Render(prefix+opts[i]) + "\n")
	}
	if end < total {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more below ···", total-end)) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("%d gateways · ★ = suggested · ↑/↓ · enter · esc", total)))
	return b.String()
}

func (m chatModel) configProviderLabels() []string {
	out := make([]string, len(m.configProviderOptions))
	for i, p := range m.configProviderOptions {
		label := strings.TrimSpace(p.DisplayName)
		if label == "" {
			label = p.ProviderID
		}
		mark := "  "
		if p.Inferred {
			mark = "★ "
		}
		out[i] = fmt.Sprintf("%s%-22s %s", mark, label, p.ProviderID)
	}
	return out
}

func (m chatModel) handleConfigKeyResolvedMsg(msg configKeyResolvedMsg) (chatModel, tea.Cmd) {
	secret := strings.TrimSpace(msg.secret)
	if !msg.result.FormatOK {
		m.configNotice = sanitizeConfigNotice(msg.result.FormatError)
		return m.startConfigEntry(configEntryAPIKeyPaste, "")
	}
	if secret == "" {
		m.configNotice = "Paste a valid API key"
		return m.startConfigEntry(configEntryAPIKeyPaste, "")
	}
	m.configPendingKey = secret
	m.configProviderOptions = msg.result.Providers
	m.configEntry = configEntryNone
	m.configMenu = configMenuProviders
	m.configTab = configTabKeys
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = "Select gateway (★ = suggested from key shape)"
	m.restoreChatInput()
	return m, nil
}

func (m chatModel) handleConfigProviderSelect() (chatModel, tea.Cmd) {
	idx := m.configSel
	if idx < 0 || idx >= len(m.configProviderOptions) {
		return m, nil
	}
	opt := m.configProviderOptions[idx]
	secret := strings.TrimSpace(m.configPendingKey)
	if secret == "" {
		m.configNotice = "Session expired — paste your API key again"
		return m.startConfigEntry(configEntryAPIKeyPaste, "")
	}
	inference := hawkconfig.InferenceFromOption(opt)
	m.configNotice = fmt.Sprintf("Validating key for %s via eyrie…", opt.DisplayName)
	m.configSaving = true
	return m, saveProviderKeyAsync(inference, secret)
}

func (m chatModel) startConfigOllamaURL() (chatModel, tea.Cmd) {
	return m.startConfigOllamaURLWithValue(configDefaultOllamaURL)
}

func (m chatModel) startConfigOllamaURLWithValue(url string) (chatModel, tea.Cmd) {
	m.configEntry = configEntryOllamaURL
	m.configProvider = configProviderOllama
	m.configMenu = configMenuNone
	if strings.TrimSpace(m.configNotice) == "" || strings.TrimSpace(m.configNotice) == "Working…" {
		m.configNotice = "Confirm Ollama URL (run: ollama serve)"
	}
	return m.startConfigURLInput(url)
}

func (m chatModel) startConfigURLInput(defaultURL string) (chatModel, tea.Cmd) {
	m.useConfigInput = true
	m.configInput.Reset()
	m.configInput.SetValue(defaultURL)
	m.configInput.Prompt = " url ❯ "
	m.configInput.Placeholder = defaultURL
	m.configInput.EchoMode = textinput.EchoNormal
	m.configInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	m.configInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
	m.configInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
	m.configInput.Focus()
	return m, textinput.Blink
}

func (m chatModel) handleConfigApplyCredentialsMsg(msg configApplyCredentialsMsg) (chatModel, tea.Cmd) {
	m.configSaving = false
	ctx := context.Background()
	if msg.err != nil {
		hawkconfig.RefreshConfigCredSnapshot(ctx)
		m.invalidateConnStatus()
		if msg.providerID == configProviderOllama {
			return m.returnToOllamaURLAfterError(msg.err)
		}
		notice := sanitizeConfigNotice(eyrieclient.FormatSetupError(msg.providerID, msg.err))
		if hawkconfig.HasConfiguredDeploymentCached(ctx) {
			notice = "Key saved — " + notice + " · retry in Gateways or Models tab"
		}
		m.configNotice = notice
		if strings.TrimSpace(m.configPendingKey) != "" && len(m.configProviderOptions) > 0 {
			m.configMenu = configMenuProviders
			m.configTab = configTabKeys
			m.configSel = 0
		} else {
			m.configMenu = configMenuNone
			m.configTab = configTabKeys
		}
		return m, nil
	}
	m.configPendingKey = ""
	m.configProviderOptions = nil
	m.configPendingOllamaURL = ""
	m.configMenu = configMenuNone
	m.configNotice = msg.summary
	InvalidateModelCache()
	m.configModelProvider = msg.providerID
	if len(msg.modelOptions) > 0 {
		modelCache[msg.providerID] = msg.modelOptions
	}
	next, cmd := m.rebuildSessionTransport()
	next.invalidateConnStatus()
	if msg.providerID == configProviderOllama {
		_ = hawkconfig.SetGlobalSetting("provider", configProviderOllama)
		next.session.SetProvider(hawkconfig.NormalizeProviderForEngine(configProviderOllama))
	}
	next.configGuideAfterKey = false
	if len(msg.modelOptions) == 0 {
		if msg.providerID == configProviderOllama {
			return next.returnToOllamaURLAfterError(fmt.Errorf("no models installed — run: ollama pull llama3.2"))
		}
		next.configTab = configTabKeys
		next.configNotice = "No models in catalog for " + msg.providerID + " — try another gateway"
		return next, cmd
	}
	next.configTab = configTabModels
	next.configSel = 0
	next.configScroll = 0
	next.configModelOptions = msg.modelOptions
	next.configNotice = "Gateway: " + msg.providerID + " — pick a model"
	return next, cmd
}

func (m chatModel) rebuildSessionTransport() (chatModel, tea.Cmd) {
	if err := eyrieclient.RebuildSessionTransport(context.Background(), m.session, m.settings, m.session.Provider()); err != nil {
		m.configNotice = sanitizeConfigNotice(err.Error())
	}
	return m, nil
}
