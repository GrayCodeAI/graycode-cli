package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/eyrie/catalog"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

// configModelOption is one row in the /config model picker (display from eyrie, id for settings).
type configModelOption struct {
	ID          string
	DisplayName string
}

// In-memory model cache per provider (avoids re-fetching on every interaction)
var modelCache = make(map[string][]configModelOption)

func fetchModelsAsync(provider string) tea.Cmd {
	return func() tea.Msg {
		models, _ := hawkconfig.FetchModelsForProvider(provider)
		opts := modelOptionsFromEntries(models)
		if len(opts) > 0 {
			modelCache[provider] = opts
		}
		return modelsFetchedMsg{options: opts, provider: provider}
	}
}

func modelOptionsFromEntries(models []catalog.ModelCatalogEntry) []configModelOption {
	var out []configModelOption
	seen := make(map[string]bool)
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		label := strings.TrimSpace(m.DisplayName)
		if label == "" {
			label = shortModelID(id)
		}
		out = append(out, configModelOption{ID: id, DisplayName: label})
	}
	return out
}

func modelOptionsFromIDs(ids []string) []configModelOption {
	compiled := hawkconfig.CompiledCatalogV1()
	out := make([]configModelOption, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		label := shortModelID(id)
		if compiled != nil {
			if model, ok := compiled.ModelsByID[id]; ok && strings.TrimSpace(model.Name) != "" {
				label = strings.TrimSpace(model.Name)
			}
		}
		out = append(out, configModelOption{ID: id, DisplayName: label})
	}
	return out
}

func loadConfigModelOptions(provider string) []configModelOption {
	provider = strings.TrimSpace(provider)
	if provider != "" {
		if cached, ok := modelCache[provider]; ok && len(cached) > 0 {
			return cached
		}
		if models, err := hawkconfig.FetchModelsForProvider(provider); err == nil && len(models) > 0 {
			return modelOptionsFromEntries(models)
		}
	}
	return modelOptionsFromIDs(hawkconfig.AllCanonicalModelIDs())
}

func configModelPickerLabels(opts []configModelOption, showProvider bool) []string {
	out := make([]string, len(opts))
	for i, opt := range opts {
		out[i] = formatModelPickerLine(opt, showProvider)
	}
	return out
}

func formatModelPickerLine(opt configModelOption, showProvider bool) string {
	label := strings.TrimSpace(opt.DisplayName)
	if label == "" {
		label = shortModelID(opt.ID)
	}
	if !showProvider {
		return label
	}
	prov := hawkconfig.ProviderOfModel(opt.ID)
	if prov == "" {
		return label
	}
	return fmt.Sprintf("%-28s %s", label, prov)
}

func shortModelID(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.LastIndex(id, "/"); i >= 0 && i < len(id)-1 {
		return id[i+1:]
	}
	return id
}

func extractModelIDs(opts []configModelOption) []string {
	out := make([]string, 0, len(opts))
	for _, o := range opts {
		if o.ID != "" {
			out = append(out, o.ID)
		}
	}
	return out
}

func configModelChoices(opts []configModelOption, showProvider bool) []string {
	if len(opts) == 0 {
		return nil
	}
	return configModelPickerLabels(opts, showProvider)
}

// /config → API keys (eyrie deployments) → eyrie ApplyCredentials → model from catalog

func (m chatModel) configOptions() []string {
	switch m.configMenu {
	case "hub":
		return m.configHubChoices()
	case "apikeys":
		return m.configDeploymentChoiceLabels()
	case "model":
		return configModelChoices(m.configModelOptions, m.configModelProvider == "")
	default:
		return nil
	}
}

func (m chatModel) configPanelView() string {
	if m.configEntry == "deployment-apikey" || m.configEntry == "provider-apikey" {
		return m.configProviderKeyView()
	}
	switch m.configMenu {
	case "hub":
		return m.configHubView()
	case "apikeys":
		return m.configDeploymentsView()
	case "deployment-detail":
		return m.configDeploymentDetailView()
	case "routing":
		return m.configRoutingView()
	case "view-config":
		return m.configViewProviderJSON()
	case "model":
		return m.configModelView()
	default:
		return ""
	}
}

func (m chatModel) configProviderKeyView() string {
	deploymentID := strings.TrimSpace(m.configProvider)
	envKey := hawkconfig.PrimaryAPIKeyEnvForDeployment(deploymentID)

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("🔑 ") + valueStyle.Render(deploymentID) + "\n")
	b.WriteString(mutedStyle.Render(envKey) + "\n\n")
	if m.useConfigInput {
		b.WriteString(m.configInput.View() + "\n")
	} else {
		b.WriteString(m.input.View() + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("enter save · esc skip") + "\n")
	return b.String()
}

const configWindowSize = 10

func (m chatModel) configModelView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	opts := m.configOptions()
	total := len(opts)

	// Ensure scroll keeps cursor visible
	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+configWindowSize {
		m.configScroll = m.configSel - configWindowSize + 1
	}

	var b strings.Builder
	title := "⚙ Select Model"
	if p := strings.TrimSpace(m.configModelProvider); p != "" {
		title = "⚙ Pick model (" + p + ")"
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")

	// Scroll up indicator
	if m.configScroll > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more above ···", m.configScroll)) + "\n")
	}

	// Visible window
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

	// Scroll down indicator
	if end < total {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  ··· %d more below ···", total-end)) + "\n")
	}

	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("%d models · ↑/↓ · enter · esc", total)))
	return b.String()
}

func (m chatModel) closeConfigPanel() chatModel {
	m.configOpen = false
	m.configMenu = ""
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = ""
	m.configEntry = ""
	m.configProvider = ""
	m.configModelOptions = nil
	m.configDeployments = nil
	m.configDeploymentID = ""
	m.configRoutingJSON = ""
	m.viewDirty = true
	m.restoreChatInput()
	return m
}

func (m *chatModel) restoreChatInput() {
	m.useConfigInput = false
	m.input.Reset()
	m.input.Prompt = "❯ "
	m.input.Placeholder = `Try "Create a PR with these changes" (Shift+Enter for newline)`
	m.input.Focus()
}

func (m chatModel) startConfigEntry(kind, provider string) (chatModel, tea.Cmd) {
	m.configEntry = kind
	m.configProvider = provider
	switch kind {
	case "deployment-apikey", "provider-apikey":
		m.useConfigInput = true
		m.configInput.Reset()
		m.configInput.Prompt = " key ❯ "
		m.configInput.Placeholder = "paste API key for " + provider
		m.configInput.EchoMode = textinput.EchoPassword
		m.configInput.EchoCharacter = '*'
		m.configInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
		m.configInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
		m.configInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
		m.configInput.Focus()
		return m, textinput.Blink
	default:
		// Use textarea for normal text entry
		m.useConfigInput = false
		m.input.Reset()
		switch kind {
		case "model":
			m.input.Prompt = " model ❯ "
			m.input.Placeholder = "model name"
		case "provider":
			m.input.Prompt = " provider ❯ "
			m.input.Placeholder = "provider name"
		}
		m.input.Focus()
		return m, m.input.Focus()
	}
}

func (m chatModel) finishConfigEntry() (chatModel, tea.Cmd) {
	var value string
	if m.useConfigInput {
		value = strings.TrimSpace(m.configInput.Value())
	} else {
		value = strings.TrimSpace(m.input.Value())
	}

	switch m.configEntry {
	case "deployment-apikey", "provider-apikey":
		deploymentID := strings.TrimSpace(m.configProvider)
		if value != "" {
			envKey := hawkconfig.PrimaryAPIKeyEnvForDeployment(deploymentID)
			if envKey != "" {
				if err := hawkconfig.PersistAPIKey(context.Background(), envKey, value); err != nil {
					m.configNotice = err.Error()
					m.configEntry = ""
					m.configMenu = "deployment-detail"
					m.restoreChatInput()
					return m, fetchDeploymentsAsync()
				}
			}
		}
		m.configEntry = ""
		m.configGuideAfterKey = true
		m.configModelProvider = hawkconfig.ProviderIDForDeployment(deploymentID)
		m.configNotice = "Applying credentials via eyrie…"
		m.restoreChatInput()
		return m, applyEyrieCredentialsAsync(deploymentID)

	case "model":
		if value == "" {
			m.configEntry = ""
			m.configProvider = ""
			m.restoreChatInput()
			return m, nil
		}
		if err := hawkconfig.SetGlobalSetting("model", value); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		} else {
			m.session.SetModel(value)
		}
		return m.closeConfigPanel(), nil

	}

	// Fallback
	m.configEntry = ""
	m.configProvider = ""
	m.restoreChatInput()
	return m, nil
}

func (m chatModel) handleConfigEntryKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.configEntry == "deployment-apikey" || m.configEntry == "provider-apikey" {
			m.configEntry = ""
			m.configProvider = ""
			m.configMenu = "apikeys"
			m.configSel = 0
			m.restoreChatInput()
			return m, nil
		}
		m.configEntry = ""
		m.configProvider = ""
		m.restoreChatInput()
		return m, nil
	case tea.KeyEnter:
		return m.finishConfigEntry()
	default:
		if m.useConfigInput {
			var cmd tea.Cmd
			m.configInput, cmd = m.configInput.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m chatModel) handleConfigKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	if m.configEntry != "" {
		return m.handleConfigEntryKey(msg)
	}
	opts := m.configOptions()
	if len(opts) == 0 && m.configMenu != "deployment-detail" && m.configMenu != "routing" {
		m.configSel = 0
		return m, nil
	}
	if len(opts) > 0 {
		if m.configSel < 0 || m.configSel >= len(opts) {
			m.configSel = 0
		}
	}

	switch msg.Type {
	case tea.KeyEsc:
		switch m.configMenu {
		case "hub", "":
			return m.closeConfigPanel(), nil
		case "deployment-detail":
			m.configMenu = "apikeys"
			m.configDeploymentID = ""
			return m, nil
		case "apikeys", "routing", "view-config":
			m.configMenu = "hub"
			m.configSel = 0
			m.configScroll = 0
			return m, nil
		case "model":
			m.configMenu = "hub"
			m.configSel = 0
			m.configScroll = 0
			m.configModelOptions = nil
			return m, nil
		default:
			return m.closeConfigPanel(), nil
		}
	case tea.KeyUp:
		if m.configMenu == "routing" {
			if m.configScroll > 0 {
				m.configScroll--
			}
			return m, nil
		}
		if len(opts) == 0 {
			return m, nil
		}
		if m.configSel == 0 {
			m.configSel = len(opts) - 1
		} else {
			m.configSel--
		}
		return m, nil
	case tea.KeyDown:
		if m.configMenu == "routing" {
			m.configScroll++
			return m, nil
		}
		if len(opts) == 0 {
			return m, nil
		}
		m.configSel = (m.configSel + 1) % len(opts)
		return m, nil
	case tea.KeyEnter:
		if m.configMenu == "deployment-detail" || m.configMenu == "routing" {
			return m, nil
		}
		if m.configSel >= 0 && m.configSel < len(opts) {
			return m.selectConfigOption(opts[m.configSel])
		}
		return m, nil
	}
	return m, nil
}

func (m chatModel) selectConfigOption(option string) (chatModel, tea.Cmd) {
	switch m.configMenu {
	case "hub":
		return m.handleConfigHubSelect(option)
	case "apikeys":
		return m.handleConfigDeploymentSelect(option)

	case "model":
		modelID := option
		if m.configSel >= 0 && m.configSel < len(m.configModelOptions) {
			modelID = m.configModelOptions[m.configSel].ID
		} else {
			modelID = hawkconfig.ResolveCanonicalModel(option)
		}
		if err := hawkconfig.SetGlobalSetting("model", modelID); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m.closeConfigPanel(), nil
		}
		m.session.SetModel(modelID)
		if prov := hawkconfig.ProviderOfModel(modelID); prov != "" {
			_ = hawkconfig.SetGlobalSetting("provider", prov)
			m.session.SetProvider(hawkconfig.NormalizeProviderForEngine(prov))
		}
		next, cmd := m.rebuildSessionTransport()
		return next.closeConfigPanel(), cmd

	default:
		return m, nil
	}
}
