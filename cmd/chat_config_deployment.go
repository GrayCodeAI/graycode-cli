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
	secret  string
	result  hawkconfig.CredentialResolveResult
}

func (m chatModel) configPanelTitle() string {
	if hawkconfig.NeedsFirstRunSetup(context.Background()) {
		return "⚙ First-time setup (eyrie)"
	}
	return "⚙ Hawk config (eyrie)"
}

// openConfigPanel: hub → paste key / Ollama / pick model.
func (m chatModel) openConfigPanel() (chatModel, tea.Cmd) {
	ctx := context.Background()
	st := hawkconfig.EvaluateSetup(ctx)
	m = m.openConfigHub(!st.HasCredentials)
	return m, nil
}

func (m chatModel) openFirstRunConfig() (chatModel, tea.Cmd) {
	return m.openConfigPanel()
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
		inference, err := eyrieclient.LocalCredentialInference("ollama")
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
		result, err := eyrieclient.ApplyEyrieCredentials(ctx)
		if err != nil {
			return configApplyCredentialsMsg{
				err:          err,
				providerID:   inference.ProviderID,
				deploymentID: inference.DeploymentID,
			}
		}

		entries, listErr := eyrieclient.ListModelsForProviderAfterApply(ctx, inference.ProviderID)
		if listErr != nil {
			return configApplyCredentialsMsg{
				err:          listErr,
				providerID:   inference.ProviderID,
				deploymentID: inference.DeploymentID,
			}
		}
		opts := configModelOptionsFromEyrie(entries)
		if len(opts) == 0 {
			fallback := eyrieclient.OptionsFromSetupUI(result, inference.ProviderID)
			opts = toConfigModelOptionsFromEyrie(fallback)
		}

		return configApplyCredentialsMsg{
			summary:      eyrieclient.FormatApplySummary(result),
			providerID:   inference.ProviderID,
			deploymentID: inference.DeploymentID,
			modelOptions: opts,
		}
	}
}

func toConfigModelOptionsFromEyrie(in []eyrieclient.ModelOption) []configModelOption {
	out := make([]configModelOption, len(in))
	for i, o := range in {
		out[i] = configModelOption{ID: o.ID, DisplayName: o.DisplayName}
	}
	return out
}

func (m chatModel) configHubView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	opts := m.configHubLabels()
	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ Connect a provider") + "\n\n")
	if notice := strings.TrimSpace(m.configNotice); notice != "" {
		b.WriteString(mutedStyle.Render(notice) + "\n\n")
	}
	for i, opt := range opts {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		b.WriteString(lineStyle.Render(prefix+opt) + "\n")
	}
	help := "↑/↓ · enter · esc close"
	if m.configSaving {
		help = "please wait…"
	}
	b.WriteString("\n" + mutedStyle.Render(help))
	return b.String()
}

func (m chatModel) handleConfigHubSelect() (chatModel, tea.Cmd) {
	if m.configSaving {
		return m, nil
	}
	opts := m.configHubOptions()
	if m.configSel < 0 || m.configSel >= len(opts) {
		return m, nil
	}
	switch opts[m.configSel].action {
	case "model":
		return m.beginConfigModelPicker()
	case "apikey":
		m.configNotice = "Paste your provider API key"
		return m.startConfigEntry("apikey-paste", "")
	case "ollama":
		return m.startConfigOllamaURL()
	default:
		return m, nil
	}
}

func (m chatModel) startConfigOllamaURL() (chatModel, tea.Cmd) {
	return m.startConfigOllamaURLWithValue("http://localhost:11434/v1")
}

func (m chatModel) startConfigOllamaURLWithValue(url string) (chatModel, tea.Cmd) {
	m.configEntry = "ollama-url"
	m.configProvider = "ollama"
	m.configMenu = ""
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

func toConfigModelOptions(in []hawkconfig.ModelOption) []configModelOption {
	out := make([]configModelOption, len(in))
	for i, o := range in {
		out[i] = configModelOption{ID: o.ID, DisplayName: o.DisplayName}
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
	b.WriteString(titleStyle.Render("🔑 Select provider (eyrie)") + "\n\n")
	if notice := strings.TrimSpace(m.configNotice); notice != "" {
		b.WriteString(mutedStyle.Render(notice) + "\n\n")
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
	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("%d providers · ★ = eyrie guess · ↑/↓ · enter · esc", total)))
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
		m.configNotice = msg.result.FormatError
		return m.startConfigEntry("apikey-paste", "")
	}
	if secret == "" {
		m.configNotice = "Paste a valid API key"
		return m.startConfigEntry("apikey-paste", "")
	}
	m.configPendingKey = secret
	m.configProviderOptions = msg.result.Providers
	m.configEntry = ""
	m.configMenu = "providers"
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = "Step 2: select provider (★ = suggested from key shape)"
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
		return m.startConfigEntry("apikey-paste", "")
	}
	inference := hawkconfig.InferenceFromOption(opt)
	m.configNotice = fmt.Sprintf("Validating key for %s via eyrie…", opt.DisplayName)
	m.configSaving = true
	return m, saveProviderKeyAsync(inference, secret)
}

func (m chatModel) handleConfigApplyCredentialsMsg(msg configApplyCredentialsMsg) (chatModel, tea.Cmd) {
	m.configSaving = false
	if msg.err != nil {
		if msg.providerID == "ollama" {
			return m.returnToOllamaURLAfterError(msg.err)
		}
		m.configNotice = formatConfigApplyError(msg.providerID, msg.err)
		if strings.TrimSpace(m.configPendingKey) != "" && len(m.configProviderOptions) > 0 {
			m.configMenu = "providers"
			m.configSel = 0
		} else {
			m.configMenu = "hub"
		}
		return m, nil
	}
	m.configPendingKey = ""
	m.configProviderOptions = nil
	m.configPendingOllamaURL = ""
	m.configNotice = msg.summary
	InvalidateModelCache()
	m.configModelProvider = msg.providerID
	if len(msg.modelOptions) > 0 {
		modelCache[msg.providerID] = msg.modelOptions
	}
	next, cmd := m.rebuildSessionTransport()
	if msg.providerID == "ollama" {
		_ = hawkconfig.SetGlobalSetting("provider", "ollama")
		next.session.SetProvider(hawkconfig.NormalizeProviderForEngine("ollama"))
	}
	next.configGuideAfterKey = false
	if len(msg.modelOptions) == 0 {
		if msg.providerID == "ollama" {
			return next.returnToOllamaURLAfterError(fmt.Errorf("no models installed — run: ollama pull llama3.2"))
		}
		next.configMenu = "hub"
		next.configNotice = "No models in catalog for " + msg.providerID + " — try another provider"
		return next, cmd
	}
	next.configMenu = "model"
	next.configSel = 0
	next.configScroll = 0
	next.configModelOptions = msg.modelOptions
	next.configNotice = "Pick a model (" + msg.providerID + ")"
	return next, cmd
}

func (m chatModel) rebuildSessionTransport() (chatModel, tea.Cmd) {
	if err := eyrieclient.RebuildSessionTransport(context.Background(), m.session, m.settings, m.session.Provider()); err != nil {
		m.configNotice = err.Error()
	}
	return m, nil
}
