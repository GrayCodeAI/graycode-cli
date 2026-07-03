package cmd

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/ui/icons"
)

// This file holds the Bubble Tea event loop for the chat TUI: the central
// Update message switch and the prompt arrow-key handler. Split out of
// chat.go so the model construction/lifecycle and the event loop live in
// separate, focused files.

// applyPromptArrowKey handles Up/Down in the prompt: slash menu navigation or input history.
// Returns true when the key was consumed so callers skip textarea/updateInput handling.
func (m *chatModel) applyPromptArrowKey(msg tea.KeyMsg) bool {
	if m.arrowBurstActive {
		// Only swallow further Up/Down here — they were already routed by
		// the burst-coalescing logic in Update(). Any other key (typing,
		// Escape, etc.) must still reach the input; arrowBurstActive is a
		// short-lived timing flag, not a general input lock.
		switch msg.Type {
		case tea.KeyUp, tea.KeyDown:
			return true
		}
		return false
	}
	if m.uiFocus != focusPrompt || m.configOpen {
		return false
	}
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown:
	default:
		return false
	}
	sugs := m.slashSuggestionsFor(m.input.Value())
	if len(sugs) > 0 {
		switch msg.Type {
		case tea.KeyUp:
			if m.slashSel <= 0 {
				m.slashSel = len(sugs) - 1
			} else {
				m.slashSel--
			}
		case tea.KeyDown:
			m.slashSel = (m.slashSel + 1) % len(sugs)
		}
		return true
	}
	switch msg.Type {
	case tea.KeyUp:
		if len(m.history) > 0 {
			if m.historyIdx == len(m.history) {
				m.historyDraft = m.input.Value()
			}
			if m.historyIdx > 0 {
				m.historyIdx--
				m.input.SetValue(m.history[m.historyIdx])
				m.input.CursorEnd()
			}
		}
		return true
	case tea.KeyDown:
		if m.historyIdx < len(m.history)-1 {
			m.historyIdx++
			m.input.SetValue(m.history[m.historyIdx])
			m.input.CursorEnd()
		} else if m.historyIdx == len(m.history)-1 {
			m.historyIdx = len(m.history)
			m.input.SetValue(m.historyDraft)
			m.input.CursorEnd()
		}
		return true
	}
	return false
}

func shouldReturnToPromptOnType(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return false
	}
	if isMouseSequenceLeak(msg) {
		return false
	}
	return true
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if _, isMouse := msg.(tea.MouseMsg); !isMouse {
		if m.refreshStatusBarLeft(false) {
			m.viewDirty = true
		}
	}

	switch msg := msg.(type) {
	case tea.FocusMsg:
		m.viewDirty = true
		m.updateViewportContent()
		if focus := m.ensurePromptInputFocus(); focus != nil {
			return m, focus
		}
		return m, nil

	case tea.BlurMsg:
		if m.uiFocus == focusPrompt && !m.configOpen && !m.useConfigInput {
			m.input.Blur()
		}
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil

	case promptKeepAliveMsg:
		if m.uiFocus == focusPrompt && !m.configOpen && !m.useConfigInput {
			if !m.input.Focused() {
				m.viewDirty = true
				m.updateViewportContent()
				return m, tea.Batch(promptKeepAliveCmd(), m.input.Focus())
			}
		}
		return m, promptKeepAliveCmd()

	case tea.MouseMsg:
		if m.mouseEnabled() {
			if m.configOpen {
				if next, handled := m.handleConfigMouse(msg); handled {
					next.viewDirty = true
					next.updateViewportContent()
					return next, nil
				}
			}
			if tea.MouseEvent(msg).IsWheel() {
				m.trackMousePosition(msg)
				cmds = append(cmds, m.applyMouseScroll(msg))
				m.sanitizeInputIfNeeded()
				m = m.syncViewportMouseWheel().withSyncedLayout()
				if m.syncInputLayout() {
					m.updateViewportContent()
				}
				if focus := m.ensurePromptInputFocus(); focus != nil {
					cmds = append(cmds, focus)
				}
			} else {
				// Motion events (?1003): track pointer only — avoid layout/sanitize/focus per move.
				m.trackMousePosition(msg)
			}
		}
		return m, tea.Batch(cmds...)

	case processArrowTickMsg:
		// A matching seq means no newer arrow keypress has arrived since this
		// tick was armed, so the burst is over — clear the flag unconditionally.
		// Without this, a burst's final keypress (which arms no tick of its
		// own) would leave arrowBurstActive stuck true forever, silently
		// swallowing every subsequent keystroke (see applyPromptArrowKey).
		if m.arrowSeq == msg.seq {
			m.arrowBurstActive = false
		}
		if m.pendingArrow != nil && m.arrowSeq == msg.seq {
			msgToProcess := *m.pendingArrow
			m.pendingArrow = nil
			m.arrowBurstActive = false
			m.processingGenuineArrow = true
			next, cmd := m.Update(msgToProcess)
			if nextModel, ok := next.(chatModel); ok {
				nextModel.processingGenuineArrow = false
				return nextModel, cmd
			}
			return next, cmd
		}
		return m, nil

	case tea.KeyMsg:
		s := msg.String()
		if (s == "up" || s == "down") && !m.processingGenuineArrow {
			now := time.Now()
			dt := now.Sub(m.lastArrowTime)
			m.lastArrowTime = now
			m.arrowSeq++
			seq := m.arrowSeq

			if dt < 30*time.Millisecond {
				m.arrowBurstActive = true
				if m.pendingArrow != nil {
					pMsg := *m.pendingArrow
					m.pendingArrow = nil
					m.processingGenuineArrow = true
					next, cmd := m.Update(pMsg)
					if nextModel, ok := next.(chatModel); ok {
						m = nextModel
					}
					m.processingGenuineArrow = false
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
				// Proceed to process `msg` immediately (fall through with m.arrowBurstActive = true).
				// Arm a trailing tick so that if this is the last keypress of
				// the burst, arrowBurstActive still gets cleared once things
				// go quiet — otherwise it would stay stuck true forever.
				cmds = append(cmds, tea.Tick(30*time.Millisecond, func(t time.Time) tea.Msg {
					return processArrowTickMsg{seq: seq}
				}))
			} else {
				m.arrowBurstActive = false
				m.pendingArrow = &msg
				return m, tea.Tick(30*time.Millisecond, func(t time.Time) tea.Msg {
					return processArrowTickMsg{seq: seq}
				})
			}
		} else {
			if m.pendingArrow != nil && !m.processingGenuineArrow {
				pMsg := *m.pendingArrow
				m.pendingArrow = nil
				m.arrowBurstActive = false
				m.processingGenuineArrow = true
				next, cmd := m.Update(pMsg)
				if nextModel, ok := next.(chatModel); ok {
					m = nextModel
				}
				m.processingGenuineArrow = false
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}

		// Ctrl+\ enters native terminal selection mode. Available in every UI
		// state (welcome gate, permissions, prompt, scrollback) so users always
		// have a way to copy text out of the chat — the alt-screen +
		// mouse-tracking combination otherwise breaks native text selection.
		if msg.Type == tea.KeyCtrlBackslash {
			return m, enterSelectionMode(m.ref, m.copyableTranscript(), m.mouseEnabled())
		}
		if isCopyToClipboardKey(msg) {
			return m.handleCopyShortcut()
		}
		if isMouseSequenceLeak(msg) {
			if m.configOpen {
				if next, handled := m.handleConfigMouseLeak(msg); handled {
					next.viewDirty = true
					next.updateViewportContent()
					return next, nil
				}
			}
			if handled, cmd := m.tryScrollFromMouseLeak(msg); handled {
				m.sanitizeInputIfNeeded()
				if focus := m.ensurePromptInputFocus(); focus != nil {
					return m, tea.Batch(cmd, focus)
				}
				return m, cmd
			}
			m.sanitizeInputIfNeeded()
			if focus := m.ensurePromptInputFocus(); focus != nil {
				return m, focus
			}
			return m, nil
		}
		// Command palette (Ctrl+K) — intercept all input when open
		if m.commandPalette != nil && m.commandPalette.IsOpen() {
			action, handled := m.commandPalette.Update(msg)
			if handled {
				if action != "" {
					// Execute the selected command
					m.commandPalette.Close()
					result, _ := m.handleCommand(action)
					if cm, ok := result.(chatModel); ok {
						m = cm
					}
					m.viewDirty = true
					m.updateViewportContent()
				}
				return m, nil
			}
		}

		// Autonomy tier picker (/autonomy) — intercept all input when open
		if m.autonomyPicker != nil && m.autonomyPicker.IsOpen() {
			chosen, handled := m.autonomyPicker.Update(msg)
			if handled {
				if chosen != nil && m.session != nil {
					m.session.PermSvc().SetAutonomy(chosen.Level)
					m.settings.Autonomy = permissionTierSettingValue(chosen.Level)
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Autonomy tier → %s\nBehavior: %s", chosen.Name, chosen.Description)})
				}
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
		}

		// Spec workflow picker (/spec) — intercept all input when open
		if m.specPicker != nil && m.specPicker.IsOpen() {
			chosen, handled := m.specPicker.Update(msg)
			if handled {
				if chosen != nil && m.session != nil {
					switch chosen.Action {
					case specActionStart:
						m.session.PermSvc().SetSpecStage(engine.SpecStageSpecify)
						m.messages = append(m.messages, displayMsg{role: "system", content: "Spec workflow started — Write/Edit/Bash are gated until spec.md, plan.md, and tasks.md are written and ApproveImplementation is approved."})
					case specActionStatus:
						m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Spec stage: %s", specStageLabel(m.session))})
					case specActionReset:
						m.session.PermSvc().SetSpecStage(engine.SpecStageNone)
						m.messages = append(m.messages, displayMsg{role: "system", content: "Spec workflow reset — Write/Edit/Bash follow the trust tier again."})
					}
				}
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
		}

		if m.manualCompacting {
			if isCompactCancelKey(msg) {
				return m.cancelManualCompact("Compaction cancelled.")
			}
			if msg.Type == tea.KeyEnter {
				return m, nil
			}
			// Allow typing in the input while compaction runs (Esc cancels).
		}

		if m.inScrollbackFocus() {
			switch msg.Type {
			case tea.KeyTab:
				return m.cycleUIFocus()
			case tea.KeyEsc:
				m.uiFocus = focusPrompt
				m.viewDirty = true
				return m, m.input.Focus()
			}
			if shouldReturnToPromptOnType(msg) {
				m.uiFocus = focusPrompt
				m.viewDirty = true
				cmds = append(cmds, m.input.Focus())
				cmds = append(cmds, m.updateInput(msg))
				m.updateViewportContent()
				return m, tea.Batch(cmds...)
			}
			if scrolled, cmd := m.applyViewportScroll(msg); scrolled {
				return m, cmd
			}
			if m.routeKeyToViewport(msg) {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				if m.viewport.AtBottom() {
					m.autoScroll = true
				} else {
					m.autoScroll = false
				}
				return m, cmd
			}
			return m, nil
		}

		if scrolled, cmd := m.applyViewportScroll(msg); scrolled {
			return m, tea.Batch(append(cmds, cmd)...)
		}

		// Permission prompt active — handle y/n
		if m.permReq != nil {
			switch msg.String() {
			case "y", "Y":
				m.permReq.Response <- true
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CheckBold() + " Allowed"})
				m.permReq = nil
			case "n", "N":
				m.permReq.Response <- false
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CloseThick() + " Denied"})
				m.permReq = nil
			case "a", "A":
				m.permReq.Response <- true
				m.session.Perm.Memory.AlwaysAllow(m.permReq.ToolName)
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CheckBold() + " Always allowed: " + m.permReq.ToolName})
				m.permReq = nil
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		}
		// AskUser prompt active — Enter submits answer
		if m.askReq != nil {
			if msg.Type == tea.KeyEnter {
				answer := strings.TrimSpace(m.input.Value())
				m.input.Reset()
				m.messages = append(m.messages, displayMsg{role: "user", content: answer})
				m.askReq.response <- answer
				m.askReq = nil
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
			return m, m.updateInput(msg)
		}
		if m.waiting {
			if msg.Type == tea.KeyCtrlC {
				// First Ctrl+C cancels stream, second quits
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
					m.streamCancelled = true
					m.messages = append(m.messages, displayMsg{role: "system", content: icons.Stop() + " Cancelled."})
					if m.partial.Len() > 0 {
						m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
						m.partial.Reset()
					}
					m.waiting = false
					m.input.Focus()
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				}
				m.saveSession()
				if m.watcherStop != nil {
					m.watcherStop()
				}
				m.quitting = true
				return m, tea.Quit
			}
			if msg.Type == tea.KeyEsc {
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
					m.streamCancelled = true
					m.messages = append(m.messages, displayMsg{role: "system", content: icons.Stop() + " Cancelled."})
					if m.partial.Len() > 0 {
						m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
						m.partial.Reset()
					}
					m.waiting = false
					m.input.Focus()
				}
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
			// Queue message on Enter while agent is working
			if msg.Type == tea.KeyEnter {
				text := strings.TrimSpace(m.input.Value())
				if text != "" {
					m.history = append(m.history, text)
					m.historyIdx = len(m.history)
					m.historyDraft = ""
					m.messageQueue = append(m.messageQueue, text)
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Queued: %s", icons.Mail(), text)})
					m.input.Reset()
					m.viewDirty = true
					m.updateViewportContent()
				}
				return m, nil
			}
			if m.applyPromptArrowKey(msg) {
				return m, tea.Batch(cmds...)
			}
			return m, m.updateInput(msg)
		}
		if m.configOpen {
			switch msg.Type {
			case tea.KeyCtrlC:
				if time.Since(m.lastCtrlC) < 1*time.Second {
					m.saveSession()
					if m.watcherStop != nil {
						m.watcherStop()
					}
					m.quitting = true
					return m, tea.Quit
				}
				m.lastCtrlC = time.Now()
				m.messages = append(m.messages, displayMsg{role: "system", content: quitAgainMsg})
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			default:
				next, cmd := m.handleConfigKey(msg)
				next.viewDirty = true
				next.updateViewportContent()
				return next, cmd
			}
		}
		switch msg.Type {
		case tea.KeyCtrlA:
			// Toggle the Agent Status HUD overlay.
			m.hudOpen = !m.hudOpen
			if m.hudOpen {
				m.hudData = m.collectHUDData()
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlK:
			// Open command palette
			if m.commandPalette == nil {
				m.commandPalette = NewCommandPalette(m.width)
			}
			m.commandPalette.Open()
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlN:
			models := configModelChoices(m.configModelOptions, false)
			if len(models) > 1 {
				current := m.session.Model()
				idx := 0
				for i, md := range models {
					if md == current {
						idx = (i + 1) % len(models)
					}
				}
				m.session.SetModel(models[idx])
				m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Model → %s", models[idx])})
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlL:
			if m.containerEnabled && !m.containerReady {
				m.messages = append(m.messages, displayMsg{role: "system", content: "Waiting for sandbox — tiers unlock when container is ready."})
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
			next := nextAutonomyTier(m.session.PermSvc().Autonomy())
			if m.session.PermSvc().Autonomy() == 0 || autonomyTierIndex(m.session.PermSvc().Autonomy()) < 0 {
				next = DefaultContainerAutonomy
			}
			m.session.PermSvc().SetAutonomy(next)
			m.invalidateConnStatus()
			m.messages = append(m.messages, displayMsg{role: "system", content: formatAutonomyTierMessage(next)})
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyCtrlC:
			if time.Since(m.lastCtrlC) < 1*time.Second {
				m.saveSession()
				if m.watcherStop != nil {
					m.watcherStop()
				}
				m.quitting = true
				return m, tea.Quit
			}
			m.lastCtrlC = time.Now()
			m.messages = append(m.messages, displayMsg{role: "system", content: quitAgainMsg})
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		case tea.KeyTab:
			// Accept ghost text suggestion if active and input is empty
			if m.ghostText.Active() && strings.TrimSpace(m.input.Value()) == "" {
				accepted := m.ghostText.Accept()
				m.input.SetValue(accepted)
				m.input.CursorEnd()
				return m, nil
			}
			sugs := m.slashSuggestionsFor(m.input.Value())
			if len(sugs) > 0 {
				if m.slashSel < 0 || m.slashSel >= len(sugs) {
					m.slashSel = 0
				}
				m.input.SetValue(applySlashSuggestion(sugs[m.slashSel]))
				m.input.CursorEnd()
				return m, nil
			}
			return m.cycleUIFocus()
		case tea.KeyUp, tea.KeyDown:
			if m.applyPromptArrowKey(msg) {
				return m, tea.Batch(cmds...)
			}
		case tea.KeyEsc:
			if len(m.slashSuggestionsFor(m.input.Value())) > 0 {
				m.slashSel = 0
				return m, nil
			}
		case tea.KeyEnter:
			return m.submitUserMessage()
		}

	case platformContextIndexMsg:
		updatePlatformContextCache(msg)
		m.invalidateConnStatus()
		m.viewDirty = true
		return m, nil

	case modelsFetchedMsg:
		m.configSaving = false
		if msg.err != nil {
			if m.configOpen {
				m.configNotice = sanitizeConfigNotice(hawkconfig.FormatConfigProviderError(msg.provider, msg.err))
				m.viewDirty = true
				m.updateViewportContent()
			}
			return m, nil
		}
		if len(msg.options) > 0 {
			m.configModelOptions = msg.options
			if msg.provider != "" {
				modelCacheMu.Lock()
				modelCache[msg.provider] = msg.options
				modelCacheMu.Unlock()
			}
			if m.configOpen && strings.Contains(m.configNotice, "Loading") {
				m.configNotice = ""
			}
		} else if m.configOpen {
			m.configNotice = hawkconfig.CatalogEmptyHint(context.Background())
		}
		if m.session != nil && msg.provider != "" {
			gw, _ := m.sessionGatewayModel()
			if gw == "" {
				gw = msg.provider
			}
			if strings.TrimSpace(gw) == strings.TrimSpace(msg.provider) {
				applyLiveModelMetadata(m.session, gw, m.session.Model())
			}
		}
		m.invalidateConnStatus()
		m.viewDirty = true
		if m.configOpen {
			if m.configTab == configTabModels {
				m = m.focusConfigActiveModelSelection()
			}
			m.updateViewportContent()
		}
		return m, nil

	case pluginRuntimeReadyMsg:
		if msg.runtime != nil {
			m.pluginRuntime = msg.runtime
			m.rebuildWelcomeCache(m.blinkClosed)
			m.viewDirty = true
			m.updateViewportContent()
		}
		return m, nil

	case startupWarmMsg:
		if strings.TrimSpace(msg.statusLeftVal) != "" {
			m.statusLeftKey = msg.statusLeftKey
			m.statusLeftVal = msg.statusLeftVal
			m.statusLeftBranch = msg.statusLeftBranch
			m.statusLeftAt = time.Now()
		}
		if msg.connStatusKey != "" || msg.connStatusVal != "" {
			m.connStatusKey = msg.connStatusKey
			m.connStatusVal = msg.connStatusVal
		}
		m.welcomeSetupState = msg.welcomeSetup
		m.welcomeAgentsOK = msg.welcomeAgentsOK
		m.rebuildWelcomeCache(m.blinkClosed)
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil

	case systemPromptContextReadyMsg:
		if contextBlock := strings.TrimSpace(msg.context); contextBlock != "" {
			m.deferredSystemContext = contextBlock
			m.deferredSystemContextReady = true
			if m.session != nil && !m.deferredSystemContextApplied {
				m.session.AppendSystemContext(contextBlock)
				m.deferredSystemContextApplied = true
			}
		}
		return m, nil

	case configApplyCredentialsMsg:
		next, cmd := m.handleConfigApplyCredentialsMsg(msg)
		if m.configOpen {
			next.viewDirty = true
			next.updateViewportContent()
		}
		return next, cmd

	case configGatewayRefreshMsg:
		next := m.handleConfigGatewayRefreshMsg(msg)
		if m.configOpen {
			next.viewDirty = true
			next.updateViewportContent()
		}
		return next, nil

	case configRemoveCredentialMsg:
		next, cmd := m.handleConfigRemoveCredentialMsg(msg)
		if m.configOpen {
			next.viewDirty = true
			next.updateViewportContent()
		}
		return next, cmd

	case loopTickMsg:
		if !m.waiting {
			result, cmd := m.handleCommand(msg.command)
			m.viewDirty = true
			m.updateViewportContent()
			return result, cmd
		}
		return m, nil

	case streamChunkMsg:
		if m.compacting && !m.manualCompacting {
			m.compacting = false
			m.brailleSpinner.SetLabel(m.spinnerVerb)
		}
		m.turnHadAssistantOutput = true
		chunk := string(msg)
		m.partial.WriteString(chunk)
		if m.turnOutputTokens == 0 {
			m.turnEstimatedOutputRunes += utf8.RuneCountInString(chunk)
		}
		if cmd := m.markPartialDirty(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.viewDirty {
			m.updateViewportContent()
		}
		return m, tea.Batch(cmds...)

	case streamRenderTickMsg:
		m.partialRenderPending = false
		if m.partialDirty {
			m.viewDirty = true
			m.partialDirty = false
			m.lastPartialRender = time.Now()
			m.updateViewportContent()
		}
		return m, nil

	case thinkingMsg:
		m.turnSawThinking = true
		return m, nil

	case streamRetryMsg:
		m.partial.Reset()
		m.turnEstimatedOutputRunes = 0
		m.messages = stripCurrentTurnThinking(m.messages)
		m.turnSawThinking = false
		m.turnHadAssistantOutput = false
		m.turnHadToolActivity = false
		m.messages = append(m.messages, displayMsg{role: "system", content: "↻ " + msg.content})
		m.viewDirty = true

	case toolUseMsg:
		m.turnHadToolActivity = true
		if m.partial.Len() > 0 {
			m.messages = append(m.messages, displayMsg{role: "assistant", content: m.partial.String()})
			m.partial.Reset()
		}
		m.messages = append(m.messages, displayMsg{role: "tool_use", content: msg.name})
		m.toolStartTime = time.Now()
		m.viewDirty = true

	case toolResultMsg:
		m.turnHadToolActivity = true
		// No "[ToolName] " prefix here — the preceding tool_use message
		// already renders the tool's name as this block's header.
		m.messages = append(m.messages, displayMsg{role: "tool_result", content: msg.content})
		m.viewDirty = true

	case blastRadiusMsg:
		m.messages = append(m.messages, displayMsg{role: "warning", content: msg.message})
		m.viewDirty = true

	case selectionResumedMsg:
		// Returned from enterSelectionMode. The terminal has been
		// restored; just trigger a redraw so the viewport reflects the
		// state that was visible before selection.
		m.viewDirty = true
		m.updateViewportContent()

	case permissionAskMsg:
		m.permReq = &msg.req
		m.permReqSeq++
		m.messages = append(m.messages, displayMsg{role: "permission", content: msg.req.Summary})
		m.viewDirty = true
		m.updateViewportContent()
		return m, permissionPromptTimeoutCmd(m.permReqSeq)

	case permissionPromptTimeoutMsg:
		if m.permReq != nil && m.permReqSeq == msg.seq {
			m.permReq = nil
			m.messages = append(m.messages, displayMsg{role: "system", content: icons.Timer() + " Permission prompt timed out."})
			m.viewDirty = true
			m.updateViewportContent()
		}
		return m, nil

	case askUserMsg:
		m.askReq = &msg
		m.askReqSeq++
		m.messages = append(m.messages, displayMsg{role: "question", content: icons.HelpCircle() + " " + msg.question})
		m.viewDirty = true
		m.input.Focus()
		m.input.SetValue("")
		m.updateViewportContent()
		return m, askUserPromptTimeoutCmd(m.askReqSeq)

	case askUserPromptTimeoutMsg:
		if m.askReq != nil && m.askReqSeq == msg.seq {
			m.askReq = nil
			m.messages = append(m.messages, displayMsg{role: "system", content: icons.Timer() + " Question timed out."})
			m.viewDirty = true
			m.updateViewportContent()
			return m, m.input.Focus()
		}
		return m, nil

	case usageUpdateMsg:
		if msg.usage != nil {
			m.turnInputTokens += msg.usage.PromptTokens
			m.turnOutputTokens += msg.usage.CompletionTokens
			m.invalidateConnStatus()
			m.viewDirty = true
		}

	case compactTickMsg:
		if m.manualCompacting {
			if m.brailleSpinner != nil {
				m.brailleSpinner.Tick()
			}
			m.viewDirty = true
			m.updateViewportContent()
			localCmds := []tea.Cmd{compactTickCmd()}
			if !m.input.Focused() {
				localCmds = append(localCmds, m.input.Focus())
			}
			return m, tea.Batch(localCmds...)
		}

	case compactDoneMsg:
		return m.finishManualCompact(msg)

	case compactStartMsg:
		if !m.manualCompacting {
			m.compacting = true
			m.brailleSpinner.SetLabel("Compacting context")
			m.viewDirty = true
		}

	case compactMsg:
		m.compacting = false
		m.brailleSpinner.SetLabel(m.spinnerVerb)
		line := fmt.Sprintf(
			"Context compacted (%s): ~%s → ~%s tokens",
			msg.strategy,
			formatHawkTokenCount(msg.tokensBefore),
			formatHawkTokenCount(msg.tokensAfter),
		)
		m.messages = append(m.messages, displayMsg{role: "system", content: line})
		m.invalidateConnStatus()
		m.viewDirty = true

	case streamDoneMsg:
		if m.streamCancelled {
			m.streamCancelled = false
			m.waiting = false
			m.cancel = nil
			m.toolStartTime = time.Time{}
			m.viewDirty = true
		}
		if m.compacting {
			m.compacting = false
			m.brailleSpinner.SetLabel(m.spinnerVerb)
		}
		m.invalidateConnStatus()
		m.flushPartialDirty()
		if m.partial.Len() > 0 {
			content := sanitizeIdentity(m.partial.String())
			m.messages = append(m.messages, displayMsg{role: "assistant", content: content})
			if m.wal != nil {
				_ = m.wal.Append(session.Message{Role: "assistant", Content: content})
			}
			// Generate ghost text suggestion from AI response
			m.ghostText.Suggest(content)
			m.partial.Reset()
		} else if m.turnSawThinking && !m.turnHadAssistantOutput && !m.turnHadToolActivity {
			// Model sent reasoning tokens but no answer — common with reasoning
			// models when the provider drops the post-reasoning content.
			m.messages = append(m.messages, displayMsg{
				role:    "error",
				content: friendlyError(fmt.Errorf("error_only_reasoning: model produced reasoning but no answer")),
			})
		}
		m.turnSawThinking = false
		m.turnHadAssistantOutput = false
		m.turnHadToolActivity = false
		m.permReq = nil
		m.askReq = nil
		m.waiting = false
		m.cancel = nil
		m.toolStartTime = time.Time{}
		m.viewDirty = true
		m.input.Focus()
		m.saveSession()

		// Process queued messages
		if len(m.messageQueue) > 0 {
			nextMsg := m.messageQueue[0]
			m.messageQueue = m.messageQueue[1:]
			m.messages = append(m.messages, displayMsg{role: "user", content: nextMsg})
			m.session.AddUser(nextMsg)
			m.waiting = true
			m.autoScroll = true
			m.viewDirty = true
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
			m.brailleSpinner.SetLabel(m.spinnerVerb)
			m.turnSawThinking = false
			m.turnHadAssistantOutput = false
			m.turnHadToolActivity = false
			m.turnInputTokens = 0
			m.turnOutputTokens = 0
			m.turnEstimatedOutputRunes = 0
			m.startedAt = time.Time{}
			m.partial.Reset()
			m.startStream()
			return m, tea.Batch(m.spinner.Tick, spinnerVerbTickCmd())
		}

	case streamErrMsg:
		m.messages = append(m.messages, displayMsg{role: "error", content: friendlyError(msg.err)})
		m.partial.Reset()
		m.permReq = nil
		m.askReq = nil
		m.waiting = false
		m.cancel = nil
		m.toolStartTime = time.Time{}
		m.viewDirty = true
		m.input.Focus()

	case spinnerVerbTickMsg:
		if !m.waiting {
			return m, tea.Batch(cmds...)
		}
		cmds = append(cmds, spinnerVerbTickCmd())
		if strings.TrimSpace(m.partial.String()) == "" {
			m.spinnerVerb = spinnerVerbs[rand.Intn(len(spinnerVerbs))]
			m.brailleSpinner.SetLabel(m.spinnerVerb)
			m.viewDirty = true
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 4)
		m.invalidateInputLayoutCache()
		m.rebuildWelcomeCache(false)
		m.viewDirty = true
		m.refreshInputLayoutIfNeeded()
		m = m.withSyncedLayout()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.waiting {
			if strings.TrimSpace(m.partial.String()) == "" {
				m.brailleSpinner.Tick()
				if m.startedAt.IsZero() {
					m.startedAt = time.Now()
				}
				m.viewDirty = true
			}
			m.displayInTok += (float64(m.tokenInputTarget()) - m.displayInTok) * 0.10
			m.displayOutTok += (float64(m.tokenOutputTarget()) - m.displayOutTok) * 0.10
		}

		if cmd != nil {
			cmds = append(cmds, cmd)
		} else if m.waiting {
			cmds = append(cmds, m.spinner.Tick)
		}
		if !m.waiting {
			return m, tea.Batch(cmds...)
		}

	case containerStatusMsg:
		m.containerStatus = msg.status
		m.containerReady = msg.ready
		m.containerErr = msg.err
		if msg.sandbox != nil {
			m.containerSandbox = msg.sandbox
			if m.session != nil {
				m.session.SetContainerExecutor(msg.sandbox)
			}
		}
		if msg.ready && m.session != nil {
			if m.session.PermSvc().Autonomy() == 0 {
				m.session.PermSvc().SetAutonomy(DefaultContainerAutonomy)
			}
			m.invalidateConnStatus()
		}
		if msg.err != nil {
			// Fall back to host mode so chat still works (container is optional).
			m.containerEnabled = false
			m.containerReady = false
			if m.session != nil {
				m.session.SetContainerRequired(false)
				m.session.SetContainerExecutor(nil)
				applyDefaultHostAutonomy(m.session)
			}
			m.messages = append(m.messages, displayMsg{
				role:    "system",
				content: "Container unavailable — running on host. " + msg.err.Error(),
			})
			m.input.Focus()
		}
		m.rebuildWelcomeCache(m.blinkClosed)
		m.viewDirty = true
		m.updateViewportContent()
	}

	if !m.waiting && m.uiFocus == focusPrompt {
		// Clear ghost text when user starts typing
		if m.ghostText.Active() && m.input.Value() != "" {
			m.ghostText.Clear()
		}
		// Vim mode key interception (operates on full textarea value)
		if m.vim != nil && m.vim.IsEnabled() {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				text := m.input.Value()
				// textarea doesn't expose cursor column; use text length as approximation
				cursor := len(text)
				newText, newCursor, consumed := m.vim.HandleKey(keyMsg, text, cursor)
				if consumed {
					if newText != text {
						m.input.SetValue(newText)
					}
					m.input.SetCursor(newCursor)
				}
				if consumed && m.vim.Mode == VimNormal {
					return m, tea.Batch(cmds...)
				}
			}
		}
		if shouldForwardToInput(msg) {
			cmds = append(cmds, m.updateInput(msg))
		}
	}
	if m.uiFocus == focusPrompt && !m.input.Focused() {
		cmds = append(cmds, m.input.Focus())
	}

	layoutChanged := m.refreshInputLayoutIfNeeded()
	if layoutChanged {
		m = m.withSyncedLayout()
	}
	if m.viewDirty || layoutChanged {
		m.updateViewportContent()
	}

	return m, tea.Batch(cmds...)
}
