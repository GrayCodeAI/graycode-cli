package cmd

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
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
			if prov := strings.TrimSpace(opt.ProviderID); prov != "" {
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
	switch m.configTab {
	case configTabGateways:
		return len(m.configGatewayRows()) + 1
	case configTabModels:
		return len(m.configFilteredModelOptions())
	}
	return 0
}

func (m chatModel) configPanelView() string {
	if m.configEntry == configEntryKeyView {
		return m.configKeyDetailView()
	}
	if m.configEntry == configEntryAPIKeyPaste {
		return m.configProviderKeyView()
	}
	if m.configEntry == configEntryOllamaURL {
		return m.configOllamaURLView()
	}
	if m.configEntry == configEntryXiaomiRegion {
		return m.configXiaomiRegionView()
	}
	if m.configEntry == configEntryZAIRegion {
		return m.configZAIRegionView()
	}
	switch m.configTab {
	case configTabGateways:
		return m.configGatewaysView()
	case configTabModels:
		return m.configModelsTabView()
	default:
		return m.configGatewaysView()
	}
}

func (m chatModel) configProviderKeyView() string {
	titleStyle := configTitleStyle()
	mutedStyle := configMutedStyle()
	providerName := strings.TrimSpace(m.configProvider)
	title := icons.Key() + " Paste API key"
	hint := "validates with provider API · stored in " + credentialsStoreLabel()
	if providerName != "" {
		title = icons.Key() + " " + hawkconfig.GatewayDisplayName(providerName)
		hint = "paste key for this gateway only · " + hint
	}
	if providerName == hawkconfig.ProviderXiaomiTokenPlan {
		reg := hawkconfig.XiaomiTokenPlanRegionLabel()
		if reg == "" {
			reg = "not set — esc and pick region with g or enter on gateway row"
		}
		hint = "region " + reg + " · tp- keys only · " + hint
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(title) + "\n")
	b.WriteString(mutedStyle.Render(hint) + "\n\n")
	if m.useConfigInput {
		b.WriteString(m.configInput.View() + "\n")
	} else {
		b.WriteString(m.input.View() + "\n")
	}
	b.WriteString("\n" + mutedStyle.Render("enter continue · esc cancel") + "\n")
	return b.String()
}

func (m chatModel) configOllamaURLView() string {
	titleStyle := configTitleStyle()
	mutedStyle := configMutedStyle()

	var b strings.Builder
	b.WriteString(titleStyle.Render(icons.Llama()+" Ollama local") + "\n")
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

const (
	configDefaultVisibleRows = 10
	configMinVisibleRows     = 4
)

// configVisibleRows uses the full-height config viewport after accounting for
// every non-list line rendered by the active tab. Scroll hints are included
// only when the current offset needs them, so a tall panel can use every row.
func (m chatModel) configVisibleRows() int {
	if m.height <= 0 {
		return configDefaultVisibleRows
	}
	reserved := m.configFixedChromeRows()
	total := len(m.configGatewayRows())
	if m.configTab == configTabModels {
		total = len(m.configFilteredModelOptions())
	}
	rows := maxInt(configMinVisibleRows, m.height-reserved)
	// Hint visibility depends on capacity, and capacity depends on hints. Two
	// iterations are sufficient because there are at most two one-line hints.
	for range 2 {
		hints := 0
		if m.configScroll > 0 {
			hints++
		}
		if m.configScroll+rows < total {
			hints++
		}
		rows = maxInt(configMinVisibleRows, m.height-reserved-hints)
	}
	return rows
}

func (m chatModel) configFixedChromeRows() int {
	// title, status, blank, tabs, divider, blank, and final help line
	rows := 7
	if notice := sanitizeConfigNotice(m.configNoticeForView()); notice != "" {
		rows += strings.Count(notice, "\n") + 2
	}
	if m.configTab == configTabModels {
		gw := strings.TrimSpace(m.configModelProvider)
		if gw == "" && m.session != nil {
			gw = strings.TrimSpace(m.session.Provider())
		}
		if gw != "" {
			rows += 2
		}
		if len(m.configModelOptions) > 0 || m.configModelSearchActive {
			rows += 2
		}
		rows += 5 // two-line header + blank + footer + capability legend
		return rows
	}
	rows += 6 // two-line header + blank + refresh + blank + selection footer
	gateways := m.configGatewayRows()
	target := m.configGatewayRefreshTargetIndex(gateways)
	if target >= 0 && target < len(gateways) && gateways[target].KeyConflict {
		rows++
	}
	return rows
}

func (m chatModel) configModelsTabView() string {
	var body strings.Builder
	gw := strings.TrimSpace(m.configModelProvider)
	if gw == "" && m.session != nil {
		gw = strings.TrimSpace(m.session.Provider())
	}
	if gw != "" {
		body.WriteString(renderConfigGatewayLine(hawkconfig.GatewayDisplayName(gw)) + "\n\n")
	}

	if len(m.configModelOptions) > 0 || m.configModelSearchActive {
		body.WriteString(m.renderConfigModelSearchLine())
		body.WriteString("\n\n")
	}
	body.WriteString(m.configModelsBody())
	return m.configTabShellView(body.String())
}

func (m chatModel) renderConfigModelSearchLine() string {
	if m.configModelSearchActive {
		return m.configInput.View()
	}
	indent := strings.Repeat(" ", modelTableIndent)
	query := strings.TrimSpace(m.configModelSearch)
	if query == "" {
		return configMutedStyle().Render(indent + "/ search models")
	}
	matches := len(m.configFilteredModelOptions())
	matchLabel := fmt.Sprintf(" · %d match", matches)
	if matches != 1 {
		matchLabel += "es"
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		configMutedStyle().Inline(true).Render(indent+"search: "),
		configAccentStyle().Inline(true).Render(query),
		configMutedStyle().Inline(true).Render(matchLabel+" · / edit"),
	)
}

func (m chatModel) startConfigModelSearch() (chatModel, tea.Cmd) {
	if m.configTab != configTabModels || len(m.configModelOptions) == 0 {
		return m, nil
	}
	m.configModelSearchActive = true
	m.useConfigInput = true
	m.configInput.Reset()
	m.configInput.SetValue(m.configModelSearch)
	m.configInput.Prompt = strings.Repeat(" ", modelTableIndent) + "search " + icons.ChevronRight() + " "
	m.configInput.Placeholder = "filter by name, owner, id"
	m.configInput.EchoMode = textinput.EchoNormal
	m.configInput.EchoCharacter = 0
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
	m.configSel = 0
	m.configScroll = 0
	return m, textinput.Blink
}

func (m chatModel) stopConfigModelSearch(clearQuery bool) chatModel {
	if m.configModelSearchActive {
		if clearQuery {
			m.configModelSearch = ""
		} else {
			m.configModelSearch = strings.TrimSpace(m.configInput.Value())
		}
	}
	m.configModelSearchActive = false
	m.useConfigInput = false
	m.configInput.Blur()
	m.configInput.Reset()
	m.configScroll = 0
	m = m.focusConfigActiveModelSelection()
	return m
}

func (m chatModel) handleConfigModelSearchKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch key := msg.Key(); key.Code {
	case tea.KeyEsc:
		return m.stopConfigModelSearch(true), nil
	case tea.KeyEnter:
		opts := m.configFilteredModelOptions()
		if m.configSel >= 0 && m.configSel < len(opts) {
			return m.selectConfigModelFromOptions(opts)
		}
		return m, nil
	case tea.KeyUp:
		n := len(m.configFilteredModelOptions())
		if n == 0 {
			return m, nil
		}
		if m.configSel == 0 {
			m.configSel = n - 1
		} else {
			m.configSel--
		}
		return m, nil
	case tea.KeyDown:
		n := len(m.configFilteredModelOptions())
		if n == 0 {
			return m, nil
		}
		m.configSel = (m.configSel + 1) % n
		return m, nil
	default:
		prev := m.configInput.Value()
		var cmd tea.Cmd
		m.configInput, cmd = m.configInput.Update(msg)
		if m.configInput.Value() != prev {
			m.configModelSearch = strings.TrimSpace(m.configInput.Value())
			m.configSel = 0
			m.configScroll = 0
		}
		return m, cmd
	}
}

func (m chatModel) configPanelViewWidth() int {
	if m.width > 0 {
		return m.width
	}
	return DetectTerminalWidth()
}

func (m chatModel) configActiveModelID() string {
	if m.session != nil {
		if modelName := strings.TrimSpace(m.session.Model()); modelName != "" {
			return modelName
		}
	}
	return strings.TrimSpace(hawkconfig.ActiveModel(context.Background()))
}

func modelOptionIsActive(opt configModelOption, activeModelID string) bool {
	return modelOptionIsActiveResolved(opt, activeModelID, hawkconfig.CanonicalModelID(context.Background(), activeModelID))
}

func modelOptionIsActiveResolved(opt configModelOption, activeModelID, activeCanonicalID string) bool {
	activeModelID = strings.TrimSpace(activeModelID)
	if activeModelID == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(opt.ID), activeModelID) {
		return true
	}
	optCanonical := strings.TrimSpace(opt.CanonicalID)
	activeCanonicalID = strings.TrimSpace(activeCanonicalID)
	if optCanonical != "" && optCanonical == activeCanonicalID {
		return true
	}
	return false
}

func (m chatModel) focusConfigActiveModelSelection() chatModel {
	opts := m.configFilteredModelOptions()
	if len(opts) == 0 {
		m.configSel = 0
		m.configScroll = 0
		return m
	}
	activeID := m.configActiveModelID()
	activeCanonicalID := hawkconfig.CanonicalModelID(context.Background(), activeID)
	windowSize := m.configVisibleRows()
	for i, opt := range opts {
		if modelOptionIsActiveResolved(opt, activeID, activeCanonicalID) {
			m.configSel = i
			if m.configSel < windowSize {
				m.configScroll = 0
			} else {
				m.configScroll = m.configSel - windowSize + 1
			}
			return m
		}
	}
	if m.configSel >= len(opts) {
		m.configSel = len(opts) - 1
	}
	if m.configSel < 0 {
		m.configSel = 0
	}
	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+windowSize {
		m.configScroll = m.configSel - windowSize + 1
	}
	return m
}

func (m chatModel) configModelsBody() string {
	mutedStyle := configMutedStyle()
	headerStyle := configHeaderStyle()
	metaStyle := configMutedStyle()
	rowStyle := configRowStyle()
	cursorStyle := configSelectedStyle()
	activeStyle := configActiveStyle()
	freeStyle := lipgloss.NewStyle().Foreground(doneGreen)

	opts := m.configFilteredModelOptions()
	total := len(opts)
	allTotal := len(m.configModelOptions)
	activeModelID := m.configActiveModelID()
	activeCanonicalID := hawkconfig.CanonicalModelID(context.Background(), activeModelID)
	windowSize := m.configVisibleRows()
	maxScroll := maxInt(0, total-windowSize)
	if m.configScroll > maxScroll {
		m.configScroll = maxScroll
	}

	if m.configSel < m.configScroll {
		m.configScroll = m.configSel
	}
	if m.configSel >= m.configScroll+windowSize {
		m.configScroll = m.configSel - windowSize + 1
	}

	end := m.configScroll + windowSize
	if end > total {
		end = total
	}

	visible := make([]modelTableRow, 0, end-m.configScroll)
	for i := m.configScroll; i < end; i++ {
		row := modelTableRowFromOption(opts[i])
		row.Active = modelOptionIsActiveResolved(opts[i], activeModelID, activeCanonicalID)
		visible = append(visible, row)
	}
	layout := computeModelTableLayout(m.configPanelViewWidth(), visible)

	var b strings.Builder
	gw := strings.TrimSpace(m.configModelProvider)
	if gw == "" && m.session != nil {
		gw = strings.TrimSpace(m.session.Provider())
	}

	if total == 0 {
		query := m.configModelSearchQuery()
		if query != "" && allTotal > 0 {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("%sNo models match %q.", strings.Repeat(" ", modelTableIndent), query)) + "\n")
			b.WriteString(mutedStyle.Render(strings.Repeat(" ", modelTableIndent)+"/ search again · esc clear") + "\n")
			return b.String()
		}
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
		row.Active = modelOptionIsActiveResolved(opts[i], activeModelID, activeCanonicalID)
		cursor := i == m.configSel
		b.WriteString(renderModelTableRow(row, cursor, row.Active, layout, rowStyle, cursorStyle, activeStyle, metaStyle, freeStyle) + "\n")
	}

	if end < total {
		b.WriteString(modelTableScrollHint(0, total-end, mutedStyle) + "\n")
	}

	b.WriteString("\n" + modelTableFooter(total, m.configScroll, end, allTotal, mutedStyle))
	b.WriteString("\n" + mutedStyle.Render(strings.Repeat(" ", modelTableIndent)+"Caps: T tools · V vision · R reasoning · J JSON · Think: t toggles on/off"))
	return b.String()
}

func (m chatModel) closeConfigPanel() chatModel {
	m.configOpen = false
	m.uiFocus = focusPrompt
	m.configTab = configTabGateways
	m.configSel = 0
	m.configScroll = 0
	m.configNotice = ""
	m.configEntry = configEntryNone
	m.configProvider = ""
	m.configPendingOllamaURL = ""
	m.configSaving = false
	m.configModelOptions = nil
	m.configModelSearch = ""
	m.configModelSearchActive = false
	m.configGatewayFocus = 0
	m.viewDirty = true
	m.restoreChatInput()
	return m
}

func (m *chatModel) restoreChatInput() {
	m.uiFocus = focusPrompt
	m.useConfigInput = false
	m.input.Reset()
	m.input.Prompt = icons.ChevronRight() + " "
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
	m.configInput.Prompt = " key " + icons.ChevronRight() + " "
	m.configInput.Placeholder = ""
	m.configInput.EchoMode = textinput.EchoPassword
	m.configInput.EchoCharacter = '*'
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
			providerName := strings.TrimSpace(m.configProvider)
			m.configEntry = configEntryNone
			m.configProvider = ""
			m.wipeConfigKeyInput()
			m.restoreChatInput()
			m.configTab = configTabGateways
			m.configNotice = "No API key entered — paste your key, then press enter"
			if providerName != "" {
				if idx := m.configGatewayRowIndex(providerName); idx >= 0 {
					m.configSel = idx
				}
			}
			return m, nil
		}
		providerName := strings.TrimSpace(m.configReplaceProvider)
		if providerName == "" {
			providerName = strings.TrimSpace(m.configProvider)
		}
		m.configReplaceProvider = ""
		if providerName == "" {
			m.configNotice = "Select a gateway on the Gateways tab first"
			m.configEntry = configEntryNone
			m.wipeConfigKeyInput()
			m.restoreChatInput()
			return m, nil
		}
		if providerName == hawkconfig.ProviderXiaomiTokenPlan && hawkconfig.NeedsXiaomiTokenPlanRegion(providerName) {
			m.configEntry = configEntryNone
			m.wipeConfigKeyInput()
			m.restoreChatInput()
			m.configPostSaveKeysProvider = providerName
			m.configNotice = "Pick Token Plan region before pasting key"
			return m.startConfigXiaomiTokenPlanRegion(), nil
		}
		m.configEntry = configEntryNone
		m.configProvider = ""
		m.wipeConfigKeyInput()
		m.restoreChatInput()
		inference, err := hawkconfig.CredentialInferenceForProvider(providerName)
		if err != nil {
			m.configTab = configTabGateways
			m.configNotice = "Could not save key: " + sanitizeConfigNotice(err.Error())
			if idx := m.configGatewayRowIndex(providerName); idx >= 0 {
				m.configSel = idx
			}
			return m, nil
		}
		m.configPostSaveKeysProvider = providerName
		m.configSaving = true
		notice := fmt.Sprintf("Validating key for %s…", inference.DisplayName)
		if hint := hawkconfig.CredentialGuidance(providerName, value); hint != "" {
			notice = hint + " · " + notice
		}
		m.configNotice = notice
		return m, saveProviderKeyAsync(inference, value)
	default:
		m.configEntry = configEntryNone
		m.restoreChatInput()
		return m, nil
	}
}

func (m chatModel) handleConfigEntryKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	switch key := msg.Key(); key.Code {
	case tea.KeyEsc:
		switch m.configEntry {
		case configEntryOllamaURL:
			m.configEntry = configEntryNone
			m.configProvider = ""
			if m.configTab == configTabGateways {
				if idx := m.configGatewayRowIndex(configProviderOllama); idx >= 0 {
					m.configSel = idx
				}
			}
			m.configNotice = ""
			m.restoreChatInput()
			return m, nil
		case configEntryAPIKeyPaste:
			providerName := strings.TrimSpace(m.configReplaceProvider)
			if providerName == "" {
				providerName = strings.TrimSpace(m.configProvider)
			}
			m.configReplaceProvider = ""
			m.configEntry = configEntryNone
			m.configProvider = ""
			m.wipeConfigKeyInput()
			m.restoreChatInput()
			if providerName != "" {
				if idx := m.configGatewayRowIndex(providerName); idx >= 0 {
					m.configSel = idx
				}
			}
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

const configWheelStep = 5

func (m chatModel) configPageStep() int {
	windowSize := m.configVisibleRows()
	if windowSize <= 2 {
		return 1
	}
	return windowSize - 1
}

func (m chatModel) configMoveSelection(delta int) chatModel {
	if delta == 0 {
		return m
	}
	n := m.configTabItemCount()
	if n <= 0 {
		m.configSel = 0
		return m
	}
	if m.configSel < 0 {
		m.configSel = 0
	} else if m.configSel >= n {
		m.configSel = n - 1
	}
	next := m.configSel + delta
	if next < 0 {
		next = 0
	} else if next >= n {
		next = n - 1
	}
	m.configSel = next
	if m.configTab == configTabGateways {
		return m.trackConfigGatewayFocus()
	}
	return m
}

func (m chatModel) handleConfigMouse(msg tea.MouseMsg) (chatModel, bool) {
	if m.configSaving {
		return m, false
	}
	switch m.configEntry {
	case configEntryNone, configEntryKeyView:
	default:
		return m, false
	}
	step := configWheelStep
	if m.configEntry == configEntryKeyView {
		step = 1
	}
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		return m.configMoveSelection(-step), true
	case tea.MouseWheelDown:
		return m.configMoveSelection(step), true
	case tea.MouseLeft:
		if _, ok := msg.(tea.MouseClickMsg); !ok || m.configEntry != configEntryNone {
			return m, false
		}
		return m.configSelectClickedRow(msg.Mouse().Y)
	default:
		return m, false
	}
}

// configSelectClickedRow maps a terminal click to an internally paginated
// gateway/model row. The outer config viewport stays pinned to the top, but
// account for its offset so this remains correct if that invariant changes.
func (m chatModel) configSelectClickedRow(terminalY int) (chatModel, bool) {
	contentY := terminalY - m.chatPaneTopY() + m.viewport.YOffset()
	firstRowY := m.configFirstVisibleRowY()
	if contentY < firstRowY {
		return m, false
	}

	total := m.configTabItemCount()
	if m.configTab == configTabGateways {
		total-- // the refresh action is positioned separately below the table
	}
	end := minInt(total, m.configScroll+m.configVisibleRows())
	visible := maxInt(0, end-m.configScroll)
	rowOffset := contentY - firstRowY
	if rowOffset >= 0 && rowOffset < visible {
		m.configSel = m.configScroll + rowOffset
		if m.configTab == configTabGateways {
			m = m.trackConfigGatewayFocus()
		}
		return m, true
	}

	if m.configTab == configTabGateways {
		refreshY := firstRowY + visible
		if end < total {
			refreshY++ // hidden-row hint below the visible table
		}
		refreshY++ // blank line before Refresh
		if contentY == refreshY {
			m.configSel = total
			return m, true
		}
	}
	return m, false
}

func (m chatModel) configFirstVisibleRowY() int {
	// Setup title, status, blank, tabs, divider, blank.
	y := 6
	if notice := sanitizeConfigNotice(m.configNoticeForView()); notice != "" {
		y += strings.Count(notice, "\n") + 2
	}
	if m.configTab == configTabModels {
		gateway := strings.TrimSpace(m.configModelProvider)
		if gateway == "" && m.session != nil {
			gateway = strings.TrimSpace(m.session.Provider())
		}
		if gateway != "" {
			y += 2
		}
		if len(m.configModelOptions) > 0 || m.configModelSearchActive {
			y += 2
		}
	}
	y += 2 // table header and rule
	if m.configScroll > 0 {
		y++
	}
	return y
}

func (m chatModel) handleConfigMouseLeak(msg tea.KeyMsg) (chatModel, bool) {
	matches := mouseSGRReportRE.FindAllStringSubmatch(msg.Key().Text, -1)
	if len(matches) == 0 {
		return m, false
	}
	handledAny := false
	for _, match := range matches {
		mouse, ok := mouseMsgFromSGRMatch(match)
		if !ok {
			continue
		}
		next, handled := m.handleConfigMouse(mouse)
		if handled {
			m = next
			handledAny = true
		}
	}
	return m, handledAny
}

func (m chatModel) handleConfigKey(msg tea.KeyMsg) (chatModel, tea.Cmd) {
	key := msg.Key()
	if m.configSaving && key.Code == tea.KeyEsc {
		if m.configTab == configTabGateways {
			return m.handleConfigGatewaysEsc(), nil
		}
		return m.closeConfigPanel(), nil
	}
	if m.configEntry == configEntryKeyView {
		if m.configSaving {
			return m, nil
		}
		return m.handleConfigKeyViewKey(msg)
	}
	if m.configEntry == configEntryXiaomiRegion {
		if m.configSaving {
			return m, nil
		}
		return m.handleConfigXiaomiRegionKey(msg)
	}
	if m.configEntry == configEntryZAIRegion {
		if m.configSaving {
			return m, nil
		}
		return m.handleConfigZAIRegionKey(msg)
	}
	if m.configEntry != configEntryNone {
		if m.configSaving {
			return m, nil
		}
		return m.handleConfigEntryKey(msg)
	}
	if m.configSaving {
		return m, nil
	}
	if m.configTab == configTabModels && m.configModelSearchActive {
		return m.handleConfigModelSearchKey(msg)
	}
	if m.configTab == configTabModels && key.Text == "/" {
		return m.startConfigModelSearch()
	}
	if m.configTab == configTabGateways {
		switch key.Text {
		case "r", "R":
			return m.refreshConfigGateway()
		case "k", "K":
			if row, ok := m.selectedConfigGateway(); ok && row.HasKey && row.ID != configProviderOllama {
				return m.startConfigKeyView(row.ID), nil
			}
		case "g", "G":
			if row, ok := m.selectedConfigGateway(); ok && row.ID == hawkconfig.ProviderXiaomiTokenPlan {
				return m.startConfigXiaomiTokenPlanRegion(), nil
			}
		}
	}
	if m.configTab == configTabModels {
		switch key.Text {
		case "t", "T":
			return m.toggleConfigModelThinking()
		}
	}
	n := m.configTabItemCount()
	if n == 0 {
		m.configSel = 0
	} else if m.configSel < 0 || m.configSel >= n {
		m.configSel = 0
	}

	switch key := msg.Key(); key.Code {
	case tea.KeyEsc:
		if m.configTab == configTabModels && strings.TrimSpace(m.configModelSearch) != "" {
			return m.stopConfigModelSearch(true), nil
		}
		if m.configTab == configTabGateways {
			return m.handleConfigGatewaysEsc(), nil
		}
		return m.closeConfigPanel(), nil
	case tea.KeyLeft:
		tab := m.configTab - 1
		if tab < configTabGateways {
			tab = configTabModels
		}
		return m.switchConfigTab(tab)
	case tea.KeyRight:
		tab := m.configTab + 1
		if tab > configTabModels {
			tab = configTabGateways
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
	case tea.KeyPgUp:
		return m.configMoveSelection(-m.configPageStep()), nil
	case tea.KeyPgDown:
		return m.configMoveSelection(m.configPageStep()), nil
	case tea.KeyHome:
		m.configSel = 0
		return m.trackConfigGatewayFocus(), nil
	case tea.KeyEnd:
		if n == 0 {
			return m, nil
		}
		m.configSel = n - 1
		return m.trackConfigGatewayFocus(), nil
	case tea.KeyDelete, tea.KeyBackspace:
		if m.configTab == configTabGateways && m.configKeysPendingRemove == "" {
			return m.handleConfigGatewaysDelete(), nil
		}
	case tea.KeyEnter:
		switch m.configTab {
		case configTabGateways:
			return m.handleConfigGatewaysSelect()
		case configTabModels:
			opts := m.configFilteredModelOptions()
			if m.configSel >= 0 && m.configSel < len(opts) {
				return m.selectConfigModelFromOptions(opts)
			}
		}
		return m, nil
	}
	return m, nil
}

func (m chatModel) selectConfigModelFromOptions(opts []configModelOption) (chatModel, tea.Cmd) {
	if m.configSel < 0 || m.configSel >= len(opts) {
		return m, nil
	}
	selected := opts[m.configSel]
	modelID := selected.ID
	if err := hawkconfig.SetGlobalSetting("model", modelID); err != nil {
		m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
		return m.closeConfigPanel(), nil
	}
	m.session.SetModel(modelID)
	if gw := strings.TrimSpace(m.configModelProvider); gw != "" {
		_ = hawkconfig.SetGlobalSetting("provider", gw)
	} else if gw := strings.TrimSpace(selected.GatewayID); gw != "" {
		_ = hawkconfig.SetGlobalSetting("provider", gw)
	} else if prov := strings.TrimSpace(selected.ProviderID); prov != "" {
		_ = hawkconfig.SetGlobalSetting("provider", prov)
	}
	m.applyModelThinkingPref(selected)
	m.syncSessionSelection()
	next, cmd := m.rebuildSessionTransport()
	next.invalidateConnStatus()
	next = next.stopConfigModelSearch(true)
	next = next.closeConfigPanel()
	if !hawkconfig.EvaluateSetupCached(context.Background()).NeedsSetup {
		next.messages = append(next.messages, displayMsg{
			role:    "setup_complete",
			content: next.session.Model(),
		})
	}
	return next, cmd
}

func (m chatModel) toggleConfigModelThinking() (chatModel, tea.Cmd) {
	opts := m.configFilteredModelOptions()
	if m.configSel < 0 || m.configSel >= len(opts) {
		return m, nil
	}
	selected := opts[m.configSel]
	if !hawkconfig.ModelCapabilitySupportsThinking(selected.Capabilities) {
		m.configNotice = "Thinking not supported for this model"
		return m, nil
	}
	settings := hawkconfig.LoadSettings()
	pref := hawkconfig.ThinkingPrefForModel(settings, selected.ID)
	// Current effective display: unset → off. Toggle flips that.
	currentlyOn := false
	if pref != nil {
		currentlyOn = *pref
	}
	next := !currentlyOn
	if err := hawkconfig.SetModelThinking(selected.ID, next); err != nil {
		m.configNotice = err.Error()
		return m, nil
	}
	label := strings.TrimSpace(selected.DisplayName)
	if label == "" {
		label = selected.ID
	}
	if next {
		m.configNotice = label + " thinking → on"
	} else {
		m.configNotice = label + " thinking → off"
	}
	// If this is the active model, apply immediately.
	if m.session != nil && modelOptionIsActiveResolved(selected, m.configActiveModelID(), hawkconfig.CanonicalModelID(context.Background(), m.configActiveModelID())) {
		m.session.SetThinkingEnabled(&next)
	}
	return m, nil
}

func (m chatModel) applyModelThinkingPref(selected configModelOption) {
	if m.session == nil {
		return
	}
	settings := hawkconfig.LoadSettings()
	provider := strings.TrimSpace(m.configModelProvider)
	if provider == "" {
		provider = strings.TrimSpace(selected.GatewayID)
	}
	if provider == "" {
		provider = strings.TrimSpace(selected.ProviderID)
	}
	m.session.SetThinkingEnabled(hawkconfig.ResolveThinkingForModel(settings, selected.ID, provider))
}
