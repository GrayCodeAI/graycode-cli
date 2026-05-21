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

func (m chatModel) configTabItemCount() int {
	switch {
	case m.configMenu == configMenuProviders:
		return len(m.configProviderOptions)
	default:
		switch m.configTab {
		case configTabKeys:
			return len(m.configKeysRows(hawkconfig.ConfiguredCredentialProviders()))
		case configTabGateways:
			return len(m.configGatewayRows()) + 1
		case configTabModels:
			return len(m.configModelOptions)
		}
	}
	return 0
}

func (m chatModel) configPanelView() string {
	if m.configEntry == configEntryAPIKeyPaste {
		return m.configProviderKeyView()
	}
	if m.configEntry == configEntryOllamaURL {
		return m.configOllamaURLView()
	}
	if m.configMenu == configMenuProviders {
		return m.configProvidersView()
	}
	switch m.configTab {
	case configTabKeys:
		return m.configKeysView()
	case configTabGateways:
		return m.configGatewaysView()
	case configTabModels:
		return m.configModelsTabView()
	default:
		return m.configKeysView()
	}
}

func (m chatModel) configProviderKeyView() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5E0E")).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8D939E"))

	var b strings.Builder
	b.WriteString(titleStyle.Render("🔑 Paste API key") + "\n")
	b.WriteString(mutedStyle.Render("eyrie validates key · pick gateway · models load from cache") + "\n\n")
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
		b.WriteString(mutedStyle.Render(sanitizeConfigNotice(notice)) + "\n\n")
	}
	if m.useConfigInput {
		b.WriteString(m.configInput.View() + "\n")
	} else {
		b.WriteString(m.input.View() + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("enter connect · esc cancel") + "\n")
	return b.String()
}

const configWindowSize = 10

func (m chatModel) configModelsTabView() string {
	return m.configTabShellView(m.configModelsBody())
}

func (m chatModel) configPanelViewWidth() int {
	if m.width > 0 {
		return m.width
	}
	return DetectTerminalWidth()
}

func (m chatModel) configModelsBody() string {
	mutedStyle := configMutedStyle()
	headerStyle := configHeaderStyle()
	metaStyle := configMutedStyle()
	selectedStyle := configSelectedStyle()
	rowStyle := configRowStyle()
	freeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6BCB77"))

	opts := m.configModelOptions
	total := len(opts)

	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+configWindowSize {
		m.configScroll = m.configSel - configWindowSize + 1
	}

	end := m.configScroll + configWindowSize
	if end > total {
		end = total
	}

	visible := make([]modelTableRow, 0, end-m.configScroll)
	for i := m.configScroll; i < end; i++ {
		visible = append(visible, modelTableRowFromOption(opts[i]))
	}
	layout := computeModelTableLayout(m.configPanelViewWidth(), visible)

	var b strings.Builder
	gw := strings.TrimSpace(m.configModelProvider)
	if gw == "" {
		gw = strings.TrimSpace(m.session.Provider())
	}
	if gw != "" {
		b.WriteString(configMutedStyle().Render("Gateway  "))
		b.WriteString(configSelectedStyle().Render(hawkconfig.GatewayDisplayName(gw)))
		b.WriteString("\n\n")
	}

	if total == 0 {
		b.WriteString(mutedStyle.Render("  No models available.") + "\n")
		if hint := hawkconfig.CatalogEmptyHint(context.Background()); hint != "" {
			b.WriteString(mutedStyle.Render("  "+hint) + "\n")
		}
		if gw == configProviderOllama {
			b.WriteString(mutedStyle.Render("  Run: ollama pull llama3.2") + "\n")
		}
		return b.String()
	}

	b.WriteString(renderModelTableHeader(layout, headerStyle, metaStyle) + "\n")

	if m.configScroll > 0 {
		b.WriteString(modelTableScrollHint(m.configScroll, 0, mutedStyle) + "\n")
	}

	for i := m.configScroll; i < end; i++ {
		row := modelTableRowFromOption(opts[i])
		b.WriteString(renderModelTableRow(row, i == m.configSel, layout, rowStyle, selectedStyle, metaStyle, freeStyle) + "\n")
	}

	if end < total {
		b.WriteString(modelTableScrollHint(0, total-end, mutedStyle) + "\n")
	}

	b.WriteString("\n" + modelTableFooter(total, m.configScroll, end, mutedStyle))
	return b.String()
}

func (m chatModel) closeConfigPanel() chatModel {
	m.configOpen = false
	m.configTab = configTabKeys
	m.configMenu = configMenuNone
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = ""
	m.configEntry = configEntryNone
	m.configProvider = ""
	m.configPendingKey = ""
	m.configProviderOptions = nil
	m.configPendingOllamaURL = ""
	m.configSaving = false
	m.configModelOptions = nil
	m.configGatewayFocus = 0
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
	if kind == configEntryOllamaURL {
		return m.startConfigOllamaURL()
	}
	if kind != configEntryAPIKeyPaste {
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
	case configEntryOllamaURL:
		if value == "" {
			value = configDefaultOllamaURL
		}
		m.configPendingOllamaURL = value
		m.configSaving = true
		m.configNotice = "Checking Ollama and discovering models…"
		m.configEntry = configEntryNone
		m.wipeConfigKeyInput()
		m.restoreChatInput()
		return m, saveOllamaAsync(value)
	case configEntryAPIKeyPaste:
		if value == "" {
			m.configEntry = configEntryNone
			m.wipeConfigKeyInput()
			m.restoreChatInput()
			return m, nil
		}
		m.configNotice = "Resolving gateways via eyrie…"
		m.configEntry = configEntryNone
		m.wipeConfigKeyInput()
		m.restoreChatInput()
		return m, resolveKeyAsync(value)
	default:
		m.configEntry = configEntryNone
		m.restoreChatInput()
		return m, nil
	}
}

func (m chatModel) handleConfigEntryKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		switch m.configEntry {
		case configEntryOllamaURL:
			m.configEntry = configEntryNone
			m.configProvider = ""
			m.configTab = configTabKeys
			m.configNotice = ""
			m.restoreChatInput()
			return m, nil
		default:
			m.configEntry = configEntryNone
			m.configProvider = ""
			m.restoreChatInput()
			return m, nil
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
	if m.configEntry != configEntryNone {
		if m.configSaving {
			return m, nil
		}
		return m.handleConfigEntryKey(msg)
	}
	if m.configSaving {
		return m, nil
	}
	n := m.configTabItemCount()
	if n == 0 {
		m.configSel = 0
	} else if m.configSel < 0 || m.configSel >= n {
		m.configSel = 0
	}

	switch msg.Type {
	case tea.KeyEsc:
		if m.configMenu == configMenuProviders {
			m.configPendingKey = ""
			m.configProviderOptions = nil
			m.configMenu = configMenuNone
			m.configTab = configTabKeys
			return m.startConfigEntry(configEntryAPIKeyPaste, "")
		}
		if m.configTab == configTabKeys {
			return m.handleConfigKeysEsc(), nil
		}
		return m.closeConfigPanel(), nil
	case tea.KeyLeft:
		if m.configMenu != configMenuNone {
			return m, nil
		}
		tab := m.configTab - 1
		if tab < configTabKeys {
			tab = configTabModels
		}
		return m.switchConfigTab(tab)
	case tea.KeyRight:
		if m.configMenu != configMenuNone {
			return m, nil
		}
		tab := m.configTab + 1
		if tab > configTabModels {
			tab = configTabKeys
		}
		return m.switchConfigTab(tab)
	case tea.KeyUp:
		if n == 0 {
			return m, nil
		}
		if m.configSel == 0 {
			m.configSel = n - 1
		} else {
			m.configSel--
		}
		return m.trackConfigGatewayFocus(), nil
	case tea.KeyDown:
		if n == 0 {
			return m, nil
		}
		m.configSel = (m.configSel + 1) % n
		return m.trackConfigGatewayFocus(), nil
	case tea.KeyEnter:
		if m.configMenu == configMenuProviders {
			return m.handleConfigProviderSelect()
		}
		switch m.configTab {
		case configTabKeys:
			return m.handleConfigKeysSelect()
		case configTabGateways:
			return m.handleConfigGatewaysSelect()
		case configTabModels:
			if m.configSel >= 0 && m.configSel < len(m.configModelOptions) {
				return m.selectConfigModel()
			}
		}
		return m, nil
	}
	return m, nil
}

func (m chatModel) selectConfigModel() (chatModel, tea.Cmd) {
	if m.configSel < 0 || m.configSel >= len(m.configModelOptions) {
		return m, nil
	}
	modelID := m.configModelOptions[m.configSel].ID
	if err := hawkconfig.SetGlobalSetting("model", modelID); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m.closeConfigPanel(), nil
	}
	m.session.SetModel(modelID)
	if gw := strings.TrimSpace(m.configModelProvider); gw != "" {
		_ = hawkconfig.SetGlobalSetting("provider", gw)
		m.session.SetProvider(hawkconfig.NormalizeProviderForEngine(gw))
	} else if prov := hawkconfig.ProviderOfModel(modelID); prov != "" {
		_ = hawkconfig.SetGlobalSetting("provider", prov)
		m.session.SetProvider(hawkconfig.NormalizeProviderForEngine(prov))
	}
	next, cmd := m.rebuildSessionTransport()
	next.invalidateConnStatus()
	next = next.closeConfigPanel()
	if !hawkconfig.EvaluateSetupCached(context.Background()).NeedsSetup {
		next.messages = append(next.messages, displayMsg{
			role:    "system",
			content: fmt.Sprintf("Setup complete — chatting with %s", next.session.Model()),
		})
	}
	return next, cmd
}
