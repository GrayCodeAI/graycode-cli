package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/GrayCodeAI/eyrie/catalog"

	hawkconfig "github.com/GrayCodeAI/hawk/config"
)

func configProviderChoices() []string {
	providers := []string{
		"anthropic", "openai", "gemini", "openrouter",
		"canopywave", "grok", "opencodego", "ollama",
	}
	var out []string
	for _, p := range providers {
		status := hawkconfig.EnvKeyStatus(p)
		var statusText string
		if p == "ollama" {
			statusText = "local"
		} else if status == "set" {
			statusText = "✓"
		} else {
			statusText = "key needed"
		}
		// Fixed-width alignment: name in 12 chars, status right-aligned
		label := fmt.Sprintf("%-12s %s", p, statusText)
		out = append(out, label)
	}
	return out
}

func configModelChoices(provider string, cached []string) []string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if len(cached) > 0 {
		out := make([]string, len(cached))
		copy(out, cached)
		return out
	}
	// Fallback: load from embedded catalog synchronously
	var out []string
	if provider != "" {
		cat := catalog.LoadModelCatalogSync("")
		for _, entry := range catalog.ModelsForProvider(&cat, provider) {
			if strings.TrimSpace(entry.ID) != "" {
				out = append(out, entry.ID)
			}
		}
	}
	sort.Strings(out)
	return out
}

func extractModelIDs(models []catalog.ModelCatalogEntry) []string {
	var out []string
	seen := make(map[string]bool)
	for _, m := range models {
		id := strings.TrimSpace(m.ID)
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// ─── Simple Config Wizard ───
// /config opens provider list → select → [key prompt] → model list → select → done

func (m chatModel) configOptions() []string {
	switch m.configMenu {
	case "provider":
		return configProviderChoices()
	case "provider-action":
		return []string{"Use this key", "Remove key"}
	case "model":
		settings := hawkconfig.LoadSettings()
		return configModelChoices(settings.Provider, m.configModels)
	default:
		return nil
	}
}

func (m chatModel) configPanelView() string {
	if m.configEntry == "provider-apikey" {
		return m.configProviderKeyView()
	}
	switch m.configMenu {
	case "provider":
		return m.configProviderView()
	case "provider-action":
		return m.configProviderActionView()
	case "model":
		return m.configModelView()
	default:
		return ""
	}
}

func (m chatModel) configProviderKeyView() string {
	provider := strings.TrimSpace(m.configProvider)
	envKey := hawkconfig.ProviderAPIKeyEnv(provider)

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("🔑 ") + valueStyle.Render(provider) + "\n")
	b.WriteString(mutedStyle.Render(envKey) + "\n\n")
	b.WriteString(m.input.View() + "\n")
	b.WriteString("\n" + mutedStyle.Render("enter save · esc skip") + "\n")
	return b.String()
}

func (m chatModel) configProviderView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ECDC4"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#e05555"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ Select Provider") + "\n\n")

	opts := m.configOptions()
	for i, opt := range opts {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		// Colorize status indicators
		if strings.Contains(opt, "✓") {
			opt = strings.Replace(opt, "✓", okStyle.Render("✓"), 1)
		} else if strings.Contains(opt, "key needed") {
			opt = strings.Replace(opt, "key needed", warnStyle.Render("key needed"), 1)
		} else if strings.Contains(opt, "local") {
			opt = strings.Replace(opt, "local", mutedStyle.Render("local"), 1)
		}
		b.WriteString(lineStyle.Render(prefix+opt) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ · enter · esc"))
	return b.String()
}

func (m chatModel) configProviderActionView() string {
	provider := strings.TrimSpace(m.configProvider)
	envKey := hawkconfig.ProviderAPIKeyEnv(provider)

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6"))
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4ECDC4"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("⚙ ") + okStyle.Render("✓") + " " + style.Render(provider) + "\n")
	b.WriteString(mutedStyle.Render(envKey) + "\n\n")

	opts := m.configOptions()
	for i, opt := range opts {
		prefix := "  "
		lineStyle := style
		if i == m.configSel {
			prefix = "❯ "
			lineStyle = selectedStyle
		}
		b.WriteString(lineStyle.Render(prefix+opt) + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("↑/↓ · enter · esc"))
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
	b.WriteString(titleStyle.Render("⚙ Select Model") + "\n\n")

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

func (m chatModel) openConfigPanel() chatModel {
	m.configOpen = true
	m.configMenu = "provider"
	m.configSel = 0
	m.configNotice = ""
	return m
}

func (m chatModel) closeConfigPanel() chatModel {
	m.configOpen = false
	m.configMenu = ""
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = ""
	m.configEntry = ""
	m.configProvider = ""
	m.configModels = nil
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
	case "provider-apikey":
		// Use textinput for password masking
		m.useConfigInput = true
		m.configInput.Reset()
		m.configInput.Prompt = " key ❯ "
		m.configInput.Placeholder = "paste " + provider + " API key"
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
	case "provider-apikey":
		provider := strings.TrimSpace(m.configProvider)
		if value != "" {
			envKey := hawkconfig.ProviderAPIKeyEnv(provider)
			if envKey != "" {
				os.Setenv(envKey, value)
				_ = hawkconfig.SaveEnvFile(envKey, value)
			}
			m.session.SetAPIKey(provider, value)
		}
		// Fetch live models from eyrie for this provider
		models, _ := hawkconfig.FetchModelsForProvider(provider)
		m.configModels = extractModelIDs(models)
		m.configEntry = ""
		m.configProvider = ""
		m.configMenu = "model"
		m.configSel = 0
		m.restoreChatInput()
		return m, nil

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

	case "provider":
		if value == "" {
			m.configEntry = ""
			m.configProvider = ""
			m.restoreChatInput()
			return m, nil
		}
		engineProvider := hawkconfig.NormalizeProviderForEngine(value)
		if err := hawkconfig.SetGlobalSetting("provider", value); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m.closeConfigPanel(), nil
		}
		m.session.SetProvider(engineProvider)

		// Same flow as normal provider selection: key prompt or model list
		if engineProvider != "ollama" && hawkconfig.EnvKeyStatus(engineProvider) != "set" {
			m.configProvider = engineProvider
			return m.startConfigEntry("provider-apikey", engineProvider)
		}
		models, _ := hawkconfig.FetchModelsForProvider(engineProvider)
		m.configModels = extractModelIDs(models)
		m.configEntry = ""
		m.configProvider = ""
		m.configMenu = "model"
		m.configSel = 0
		m.restoreChatInput()
		return m, nil
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
		if m.configEntry == "provider-apikey" {
			// Skip key entry, go to model selection
			m.configEntry = ""
			m.configProvider = ""
			m.configMenu = "model"
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
	if len(opts) == 0 {
		m.configSel = 0
		return m, nil
	}
	if m.configSel < 0 || m.configSel >= len(opts) {
		m.configSel = 0
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.configMenu == "provider" || m.configMenu == "" {
			return m.closeConfigPanel(), nil
		}
		if m.configMenu == "provider-action" {
			m.configProvider = ""
			m.configMenu = "provider"
			m.configSel = 0
			return m, nil
		}
		// From model list → back to provider list
		m.configMenu = "provider"
		m.configSel = 0
		m.configNotice = ""
		m.configModels = nil
		return m, nil
	case tea.KeyUp:
		if m.configSel == 0 {
			m.configSel = len(opts) - 1
		} else {
			m.configSel--
		}
		return m, nil
	case tea.KeyDown:
		m.configSel = (m.configSel + 1) % len(opts)
		return m, nil
	case tea.KeyEnter:
		return m.selectConfigOption(opts[m.configSel])
	}
	return m, nil
}

func (m chatModel) selectConfigOption(option string) (chatModel, tea.Cmd) {
	switch m.configMenu {
	case "provider":
		// Extract provider name (first word) and normalize for engine
		provider := strings.Fields(option)[0]
		engineProvider := hawkconfig.NormalizeProviderForEngine(provider)
		if err := hawkconfig.SetGlobalSetting("provider", provider); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m.closeConfigPanel(), nil
		}
		m.session.SetProvider(engineProvider)

		// Seamless flow
		if engineProvider == "ollama" {
			// Local provider → straight to models
			models, _ := hawkconfig.FetchModelsForProvider(engineProvider)
			m.configModels = extractModelIDs(models)
			m.configMenu = "model"
			m.configSel = 0
			return m, nil
		}
		if hawkconfig.EnvKeyStatus(engineProvider) != "set" {
			// Key missing → prompt for it
			m.configProvider = engineProvider
			return m.startConfigEntry("provider-apikey", engineProvider)
		}
		// Key is set → show action menu (use or remove)
		m.configProvider = engineProvider
		m.configMenu = "provider-action"
		m.configSel = 0
		return m, nil

	case "provider-action":
		provider := strings.TrimSpace(m.configProvider)
		switch option {
		case "Use this key":
			models, _ := hawkconfig.FetchModelsForProvider(provider)
			m.configModels = extractModelIDs(models)
			m.configMenu = "model"
			m.configSel = 0
			return m, nil
		case "Remove key":
			envKey := hawkconfig.ProviderAPIKeyEnv(provider)
			if envKey != "" {
				os.Unsetenv(envKey)
				_ = hawkconfig.RemoveEnvFile(envKey)
			}
			m.configProvider = ""
			m.configMenu = "provider"
			m.configSel = 0
			return m, nil
		}
		return m, nil

	case "model":
		if err := hawkconfig.SetGlobalSetting("model", option); err != nil {
			m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
			return m.closeConfigPanel(), nil
		}
		m.session.SetModel(option)
		return m.closeConfigPanel(), nil

	default:
		return m, nil
	}
}
