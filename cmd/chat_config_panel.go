package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func configModelChoices(opts []configModelOption, showProvider bool) []string {
	if len(opts) == 0 {
		return nil
	}
	out := make([]string, len(opts))
	for i, opt := range opts {
		label := strings.TrimSpace(opt.DisplayName)
		if label == "" {
			label = shortModelID(opt.ID)
		}
		if showProvider {
			if prov := hawkconfig.ProviderOfModel(opt.ID); prov != "" {
				label = fmt.Sprintf("%-28s %s", label, prov)
			}
		}
		out[i] = label
	}
	return out
}

func shortModelID(id string) string {
	id = strings.TrimSpace(id)
	if i := strings.LastIndex(id, "/"); i >= 0 && i < len(id)-1 {
		return id[i+1:]
	}
	return id
}

// /config → paste key → all providers (eyrie) → model from catalog

func (m chatModel) configOptions() []string {
	switch m.configMenu {
	case "hub":
		return m.configHubLabels()
	case "providers":
		return m.configProviderLabels()
	case "remove-key":
		return m.configRemoveKeyLabels()
	case "model":
		return configModelChoices(m.configModelOptions, m.configModelProvider == "")
	default:
		return nil
	}
}

func (m chatModel) configPanelView() string {
	if m.configEntry == "apikey-paste" {
		return m.configProviderKeyView()
	}
	if m.configEntry == "ollama-url" {
		return m.configOllamaURLView()
	}
	switch m.configMenu {
	case "hub":
		return m.configHubView()
	case "providers":
		return m.configProvidersView()
	case "remove-key":
		return m.configRemoveKeyView()
	case "model":
		return m.configModelView()
	default:
		return ""
	}
}

func (m chatModel) configProviderKeyView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("🔑 Paste API key") + "\n")
	b.WriteString(mutedStyle.Render("eyrie validates key · you pick provider · dynamic models") + "\n\n")
	if m.useConfigInput {
		b.WriteString(m.configInput.View() + "\n")
	} else {
		b.WriteString(m.input.View() + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("enter continue · esc cancel") + "\n")
	return b.String()
}

func (m chatModel) configOllamaURLView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("🦙 Ollama local") + "\n")
	b.WriteString(mutedStyle.Render("no API key · eyrie discovers installed models") + "\n\n")
	if notice := strings.TrimSpace(m.configNotice); notice != "" {
		b.WriteString(mutedStyle.Render(notice) + "\n\n")
	}
	if m.useConfigInput {
		b.WriteString(m.configInput.View() + "\n")
	} else {
		b.WriteString(m.input.View() + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("enter connect · esc back") + "\n")
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
	if notice := strings.TrimSpace(m.configNotice); notice != "" {
		b.WriteString(mutedStyle.Render(notice) + "\n\n")
	}

	if total == 0 {
		b.WriteString(mutedStyle.Render("  No models available.") + "\n")
		if hint := hawkconfig.CatalogEmptyHint(context.Background()); hint != "" {
			b.WriteString(mutedStyle.Render("  "+hint) + "\n")
		}
		if m.configModelProvider == "ollama" {
			b.WriteString(mutedStyle.Render("  Run: ollama pull llama3.2") + "\n")
		}
		b.WriteString("\n" + mutedStyle.Render("esc → change provider"))
		return b.String()
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
	m.configPendingKey = ""
	m.configProviderOptions = nil
	m.configPendingOllamaURL = ""
	m.configSaving = false
	m.configModelOptions = nil
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
	if kind == "ollama-url" {
		return m.startConfigOllamaURL()
	}
	if kind != "apikey-paste" {
		return m, nil
	}
	m.useConfigInput = true
	m.configInput.Reset()
	m.configInput.Prompt = " key ❯ "
	m.configInput.Placeholder = "paste API key"
	m.configInput.EchoMode = textinput.EchoPassword
	m.configInput.EchoCharacter = '*'
	m.configInput.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	m.configInput.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F2F2F2"))
	m.configInput.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E"))
	m.configInput.Focus()
	return m, textinput.Blink
}

func (m chatModel) finishConfigEntry() (chatModel, tea.Cmd) {
	value := strings.TrimSpace(m.configInput.Value())
	switch m.configEntry {
	case "ollama-url":
		if value == "" {
			value = "http://localhost:11434/v1"
		}
		m.configPendingOllamaURL = value
		m.configSaving = true
		m.configNotice = "Checking Ollama and discovering models…"
		m.configEntry = ""
		m.restoreChatInput()
		return m, saveOllamaAsync(value)
	case "apikey-paste":
		if value == "" {
			m.configEntry = ""
			m.restoreChatInput()
			return m, nil
		}
		m.configNotice = "Resolving providers via eyrie…"
		m.configEntry = ""
		m.restoreChatInput()
		return m, resolveKeyAsync(value)
	default:
		m.configEntry = ""
		m.restoreChatInput()
		return m, nil
	}
}

func (m chatModel) handleConfigEntryKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		switch m.configEntry {
		case "ollama-url":
			m.configEntry = ""
			m.configProvider = ""
			m.configMenu = "hub"
			m.configSel = 1
			m.configNotice = "Step 1: choose how to connect"
			m.restoreChatInput()
			return m, nil
		default:
			m.configEntry = ""
			m.configProvider = ""
			m.restoreChatInput()
			return m.closeConfigPanel(), nil
		}
	case tea.KeyEnter:
		return m.finishConfigEntry()
	default:
		var cmd tea.Cmd
		m.configInput, cmd = m.configInput.Update(msg)
		return m, cmd
	}
}

func (m chatModel) handleConfigKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	if m.configEntry != "" {
		if m.configSaving {
			return m, nil
		}
		return m.handleConfigEntryKey(msg)
	}
	if m.configSaving {
		return m, nil
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
		switch m.configMenu {
		case "providers":
			m.configPendingKey = ""
			m.configProviderOptions = nil
			return m.startConfigEntry("apikey-paste", "")
		case "model":
			m.configMenu = "hub"
			m.configSel = 0
			m.configNotice = m.configHubNotice()
			m.restoreChatInput()
			return m, nil
		case "remove-key":
			m.configMenu = "hub"
			m.configSel = 0
			m.configNotice = m.configHubNotice()
			m.restoreChatInput()
			return m, nil
		case "hub":
			return m.closeConfigPanel(), nil
		default:
			return m.closeConfigPanel(), nil
		}
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
		switch m.configMenu {
		case "hub":
			return m.handleConfigHubSelect()
		case "providers":
			return m.handleConfigProviderSelect()
		case "remove-key":
			return m.handleConfigRemoveKeySelect()
		case "model":
			if m.configSel >= 0 && m.configSel < len(opts) {
				return m.selectConfigOption(opts[m.configSel])
			}
		}
		return m, nil
	}
	return m, nil
}

func (m chatModel) selectConfigOption(option string) (chatModel, tea.Cmd) {
	if m.configMenu != "model" {
		return m, nil
	}
	var modelID string
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
	} else if p := strings.TrimSpace(m.configModelProvider); p != "" {
		_ = hawkconfig.SetGlobalSetting("provider", p)
		m.session.SetProvider(hawkconfig.NormalizeProviderForEngine(p))
	}
	next, cmd := m.rebuildSessionTransport()
	next = next.closeConfigPanel()
	if !hawkconfig.EvaluateSetup(context.Background()).NeedsSetup {
		next.messages = append(next.messages, displayMsg{
			role:    "system",
			content: fmt.Sprintf("Setup complete — chatting with %s", next.session.Model()),
		})
	}
	return next, cmd
}
