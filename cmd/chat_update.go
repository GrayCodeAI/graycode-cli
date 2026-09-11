package cmd

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
	"github.com/GrayCodeAI/hawk/internal/engine"
	"github.com/GrayCodeAI/hawk/internal/session"
	"github.com/GrayCodeAI/hawk/internal/spec"
	"github.com/GrayCodeAI/hawk/internal/tool"
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
		switch msg.Key().Code {
		case tea.KeyUp, tea.KeyDown:
			return true
		}
		return false
	}
	if m.uiFocus != focusPrompt || m.configOpen {
		return false
	}
	switch msg.Key().Code {
	case tea.KeyUp, tea.KeyDown:
	default:
		return false
	}
	sugs := m.slashSuggestionsFor(m.input.Value())
	if len(sugs) > 0 {
		switch msg.Key().Code {
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
	switch msg.Key().Code {
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
	if len(msg.Key().Text) == 0 {
		return false
	}
	if isMouseSequenceLeak(msg) {
		return false
	}
	return true
}

// quitModel performs the shared graceful-quit sequence used by every exit
// path (Ctrl+C twice, /quit, SIGINT as tea.InterruptMsg, SIGTERM/SIGHUP as
// tea.QuitMsg): cancel any in-flight stream, persist the session, stop
// background workers (watcher, parallel agents, background tasks), stop the
// sandbox container, and mark the model as quitting so the final view can
// show the resume hint.
func (m *chatModel) quitModel() (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	saveInputHistory(m.history)
	m.saveSession()
	if m.watcherStop != nil {
		m.watcherStop()
	}
	if m.parallelCancel != nil {
		m.parallelCancel()
	}
	if m.bgCancel != nil {
		m.bgCancel()
	}
	m.stopContainer()
	m.quitting = true
	return m, tea.Quit
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if _, isMouse := msg.(tea.MouseMsg); !isMouse {
		if changed, prCmd := m.refreshStatusBarLeft(false); changed {
			m.viewDirty = true
			if prCmd != nil {
				cmds = append(cmds, prCmd)
			}
		}
	}

	switch msg := msg.(type) {
	case sessionSaveResultMsg:
		if msg.err != nil {
			m.recordWALError(msg.err)
			return m, nil
		}
		// Remove the WAL only when no append happened after the snapshot.
		// Doing this in the background save closure could race with a new
		// submission and lose messages that were not in the saved session.
		if msg.id == m.sessionID && msg.seq == m.walSeq && m.wal != nil {
			if err := m.wal.Remove(); err != nil {
				m.recordWALError(err)
			} else {
				m.wal = nil
			}
		}
		return m, nil
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
		// Track that the terminal lost focus during a turn so we can notify
		// the user when the agent completes.
		if m.waiting {
			m.backgrounded = true
		}
		m.viewDirty = true
		m.updateViewportContent()
		return m, nil

	case tea.InterruptMsg:
		// External SIGINT delivered while the terminal is not in raw mode
		// (e.g. `kill -INT`, tmux/screen `prefix` + ctrl+c). Bubble Tea would
		// otherwise exit without saving the session.
		return m.quitModel()

	case tea.QuitMsg:
		// SIGTERM (e.g. `kill <pid>`, terminal close on some platforms).
		// Exit through the same save-and-cleanup path as Ctrl+C.
		return m.quitModel()

	case promptKeepAliveMsg:
		if m.uiFocus == focusPrompt && !m.configOpen && !m.useConfigInput {
			if !m.input.Focused() {
				m.viewDirty = true
				m.updateViewportContent()
				return m, tea.Batch(promptKeepAliveCmd(), m.input.Focus())
			}
		}
		return m, promptKeepAliveCmd()

	case statusLeftPRsMsg:
		// Async PR lookup result — only apply if we're still on the same branch.
		if m.statusLeftBranch == msg.branch {
			m.statusLeftPRs = msg.nums
			m.viewDirty = true
			m.updateViewportContent()
		}
		return m, nil

	case eyeBlinkTickMsg:
		if m.showWelcomeBanner() {
			m.eyeFrame = 1
			m.rebuildWelcomeCache()
			if len(m.messages) > 0 && m.messages[0].role == "welcome" {
				m.messages[0].content = m.welcomeCache
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, tea.Batch(eyeBlinkTickCmd(), eyeFrameNextCmd(2, 60*time.Millisecond))
		}
		return m, eyeBlinkTickCmd()

	case eyeFrameNextMsg:
		if m.showWelcomeBanner() {
			m.eyeFrame = msg.frame
			m.rebuildWelcomeCache()
			if len(m.messages) > 0 && m.messages[0].role == "welcome" {
				m.messages[0].content = m.welcomeCache
			}
			m.viewDirty = true
			m.updateViewportContent()
			switch msg.frame {
			case 2:
				return m, eyeFrameNextCmd(3, 100*time.Millisecond)
			case 3:
				return m, eyeFrameNextCmd(0, 60*time.Millisecond)
			}
		} else {
			m.eyeFrame = 0
		}
		return m, nil

	case tea.MouseMsg:
		if m.mouseEnabled() {
			if m.configOpen {
				if next, handled := m.handleConfigMouse(msg); handled {
					next.viewDirty = true
					next.updateViewportContent()
					return next, nil
				}
			}
			if msg.Mouse().Button == tea.MouseWheelUp || msg.Mouse().Button == tea.MouseWheelDown {
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

	case tea.PasteMsg:
		if m.configOpen {
			var cmd tea.Cmd
			m.configInput, cmd = m.configInput.Update(msg)
			m.viewDirty = true
			m.updateViewportContent()
			return m, cmd
		}
		if m.uiFocus == focusPrompt {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.viewDirty = true
			m.updateViewportContent()
			return m, cmd
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
		// Input history search (Ctrl+R) — intercept all input when open.
		if m.historySearchOpen {
			switch msg.String() {
			case "ctrl+c", "ctrl+g", "escape":
				m.historySearchOpen = false
				m.historySearchInput = ""
				m.historySearchQuery = ""
				m.historySearchFiltered = nil
				m.historySearchSel = 0
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			case "enter":
				if len(m.historySearchFiltered) > 0 && m.historySearchSel < len(m.historySearchFiltered) {
					m.input.SetValue(m.historySearchFiltered[m.historySearchSel])
					m.input.CursorEnd()
				}
				m.historySearchOpen = false
				m.historySearchInput = ""
				m.historySearchQuery = ""
				m.historySearchFiltered = nil
				m.historySearchSel = 0
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			case "up":
				if len(m.historySearchFiltered) > 0 {
					m.historySearchSel--
					if m.historySearchSel < 0 {
						m.historySearchSel = len(m.historySearchFiltered) - 1
					}
					m.viewDirty = true
				}
				return m, nil
			case "down":
				if len(m.historySearchFiltered) > 0 {
					m.historySearchSel = (m.historySearchSel + 1) % len(m.historySearchFiltered)
					m.viewDirty = true
				}
				return m, nil
			default:
				// Forward printable characters to the search query.
				if msg.Key().Text != "" && len(msg.Key().Text) > 0 {
					// Only accept single rune input, not modifier combos.
					if msg.Key().Mod == 0 && msg.Key().Code == 0 {
						m.historySearchInput += msg.Key().Text
						m.applyHistorySearchFilter()
						m.viewDirty = true
						return m, nil
					}
				}
				// Backspace handling.
				if msg.Key().Code == tea.KeyBackspace || msg.String() == "backspace" {
					if len(m.historySearchInput) > 0 {
						runes := []rune(m.historySearchInput)
						m.historySearchInput = string(runes[:len(runes)-1])
						m.applyHistorySearchFilter()
						m.viewDirty = true
					}
					return m, nil
				}
				return m, nil
			}
		}

		// Session picker (Ctrl+S) — intercept all input when open.
		if m.sessionPickerOpen {
			switch msg.String() {
			case "ctrl+c", "ctrl+g", "escape":
				m.sessionPickerOpen = false
				m.sessionPickerInput = ""
				m.sessionPickerEntries = nil
				m.sessionPickerFiltered = nil
				m.sessionPickerSel = 0
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			case "ctrl+s":
				// Press Ctrl+S again to close.
				m.sessionPickerOpen = false
				m.sessionPickerInput = ""
				m.sessionPickerEntries = nil
				m.sessionPickerFiltered = nil
				m.sessionPickerSel = 0
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			case "enter":
				if len(m.sessionPickerFiltered) > 0 && m.sessionPickerSel < len(m.sessionPickerFiltered) {
					selected := m.sessionPickerFiltered[m.sessionPickerSel]
					// Close picker first.
					m.sessionPickerOpen = false
					m.sessionPickerInput = ""
					m.sessionPickerEntries = nil
					m.sessionPickerFiltered = nil
					m.sessionPickerSel = 0
					// Load the selected session.
					return m.resumeSessionByID(selected.ID)
				}
				return m, nil
			case "up":
				if len(m.sessionPickerFiltered) > 0 {
					m.sessionPickerSel--
					if m.sessionPickerSel < 0 {
						m.sessionPickerSel = len(m.sessionPickerFiltered) - 1
					}
					m.viewDirty = true
				}
				return m, nil
			case "down":
				if len(m.sessionPickerFiltered) > 0 {
					m.sessionPickerSel = (m.sessionPickerSel + 1) % len(m.sessionPickerFiltered)
					m.viewDirty = true
				}
				return m, nil
			default:
				// Forward printable characters to the search query.
				if msg.Key().Text != "" && len(msg.Key().Text) > 0 {
					if msg.Key().Mod == 0 && msg.Key().Code == 0 {
						m.sessionPickerInput += msg.Key().Text
						m.applySessionPickerFilter()
						m.viewDirty = true
						return m, nil
					}
				}
				// Backspace handling.
				if msg.Key().Code == tea.KeyBackspace || msg.String() == "backspace" {
					if len(m.sessionPickerInput) > 0 {
						runes := []rune(m.sessionPickerInput)
						m.sessionPickerInput = string(runes[:len(runes)-1])
						m.applySessionPickerFilter()
						m.viewDirty = true
					}
					return m, nil
				}
				return m, nil
			}
		}

		// Command palette (Ctrl+K) — intercept all input when open.
		// Must come before the Ctrl+K selection-mode handler so the
		// palette can receive Ctrl+K for navigation/close.
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

		// Ctrl+K enters native terminal selection mode. Available in every UI
		// state (welcome gate, permissions, prompt, scrollback) so users always
		// have a way to copy text out of the chat — the alt-screen +
		// mouse-tracking combination otherwise breaks native text selection.
		// Placed AFTER the command palette check so an open palette receives
		// Ctrl+K for navigation/close instead of entering selection mode.
		if msg.String() == "ctrl+k" {
			return m, enterSelectionMode(m.ref, m.copyableTranscript(), m.mouseEnabled())
		}

		// Autonomy tier picker (/autonomy) — intercept all input when open
		if m.autonomyPicker != nil && m.autonomyPicker.IsOpen() {
			chosen, handled := m.autonomyPicker.Update(msg)
			if handled {
				if chosen != nil && m.session != nil {
					// YOLO ("Autonomous") is unattended mode: require a typed
					// confirmation instead of a single Enter, so a stray key
					// cannot silently drop the session into never-ask.
					if chosen.Level == engine.AutonomyYOLO {
						m.pendingYOLOConfirm = true
						m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Autonomy tier → %s — this enables unattended mode (never prompts for permission). Type %s then Enter to confirm, or type anything else to cancel.", chosen.Name, yoloConfirmToken)})
						m.viewDirty = true
						m.updateViewportContent()
						return m, nil
					}
					m.session.PermSvc().SetAutonomy(chosen.Level)
					m.settings.Autonomy = permissionTierSettingValue(chosen.Level)
					m.settings.AutonomyExplicit = true
					m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Autonomy tier → %s\nBehavior: %s", chosen.Name, chosen.Description)})
				}
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
			}
		}

		// Theme picker (/theme) — intercept all input when open
		if m.themePicker != nil && m.themePicker.IsOpen() {
			chosen, handled := m.themePicker.Update(msg)
			if handled {
				if chosen != nil {
					if err := hawkconfig.SetGlobalSetting("theme", chosen.Name); err != nil {
						m.messages = append(m.messages, displayMsg{role: "error", content: err.Error()})
					} else {
						// Apply immediately — repaints with full palette on next frame.
						ApplyTheme(chosen.Name)
						m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("%s Theme set to: %s", icons.CheckBold(), chosen.Name)})
					}
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
						m.session.PermSvc().SetSpecStage(engine.SpecStageProposal)
						m.messages = append(m.messages, displayMsg{role: "system", content: "Spec workflow started — Write/Edit/Bash are gated. Start with Proposal, then Specify + Design (parallel), then Plan, Tasks, and ApproveImplementation."})
					case specActionStatus:
						m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Spec stage: %s", specStageLabel(m.session))})
					case specActionEdit:
						m.messages = append(m.messages, displayMsg{role: "system", content: "Use the SpecEdit tool to modify spec artifacts (spec.md, plan.md, tasks.md). You can apply deltas or replace content entirely."})
					case specActionResume:
						stage := currentSpecStage(m.session)
						stageName := specStageDisplayName(stage)
						m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Resuming from %s — continue working through the spec workflow.", stageName)})
					case specActionArchive:
						m.messages = append(m.messages, displayMsg{role: "system", content: "Archive a completed spec. The agent will use the ArchiveSpec tool to archive the spec when implementation is complete."})
					case specActionConfigure:
						cfg := spec.LoadSpecConfig()
						msg := "Spec configuration:\n" + cfg.Format()
						msg += "\n\nUse `/spec config set <field> <value>` to change settings."
						msg += "\nThe agent can also use `SpecConfig` tool to read/update."
						m.messages = append(m.messages, displayMsg{role: "system", content: msg})
					case specActionReset:
						m.session.PermSvc().ResetSpec()
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
			if msg.String() == "enter" {
				return m, nil
			}
			// Allow typing in the input while compaction runs (Esc cancels).
		}

		if m.inScrollbackFocus() {
			switch msg.Key().Code {
			case tea.KeyTab:
				return m.cycleUIFocus()
			case tea.KeyEsc:
				m.uiFocus = focusPrompt
				m.viewDirty = true
				return m, m.input.Focus()
			case tea.KeyEnter:
				// Toggle expansion of the tool result nearest the viewport center.
				if idx := m.toolResultIndexAtViewportCenter(m.width); idx >= 0 {
					m.toolResultExpanded[idx] = !m.toolResultExpanded[idx]
					m.invalidateViewportCache()
					m.viewDirty = true
					m.updateViewportContent()
				}
				return m, nil
			}
			// 'c' copies the code block nearest to viewport center.
			if msg.String() == "c" {
				content, ok := m.codeBlockAtViewportCenter()
				if !ok {
					m.messages = append(m.messages, displayMsg{
						role:    "system",
						content: "No code block found at viewport center. Scroll to a code block and press 'c' again.",
					})
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				}
				result := copyToClipboard(content)
				lineCount := strings.Count(content, "\n") + 1
				if result.FallbackPath != "" {
					// Clipboard unavailable — saved to file.
					m.messages = append(m.messages, displayMsg{
						role:    "system",
						content: fmt.Sprintf("Clipboard unavailable — saved code block (%d lines) to %s", lineCount, result.FallbackPath),
					})
				} else {
					// Show a brief highlight message with line count.
					m.messages = append(m.messages, displayMsg{
						role:    "system",
						content: fmt.Sprintf("%s Copied code block (%d lines) to clipboard.", icons.CheckBold(), lineCount),
					})
				}
				m.viewDirty = true
				m.updateViewportContent()
				return m, nil
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

		// Permission prompt active — handle y/n/a/d
		if m.permReq != nil {
			switch msg.String() {
			case "y", "Y":
				req := m.permReq
				req.Response <- true
				m.permReq = nil
				m.permTimeoutAt = time.Time{}
				if m.session != nil && m.session.PermSvc() != nil && m.session.PermSvc().AutoMode() != nil {
					m.session.PermSvc().AutoMode().Record(req.ToolName, req.Summary, true)
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CheckBold() + " Allowed"})
			case "n", "N":
				req := m.permReq
				req.Response <- false
				m.permReq = nil
				m.permTimeoutAt = time.Time{}
				if m.session != nil && m.session.PermSvc() != nil && m.session.PermSvc().AutoMode() != nil {
					m.session.PermSvc().AutoMode().Record(req.ToolName, req.Summary, false)
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CloseThick() + " Denied"})
			case "a", "A":
				req := m.permReq
				toolName := req.ToolName
				summary := req.Summary
				req.Response <- true
				m.permReq = nil
				m.permTimeoutAt = time.Time{}
				if m.session != nil && m.session.PermSvc() != nil {
					if mem := m.session.PermSvc().Memory(); mem != nil {
						mem.AlwaysAllowPattern(toolName + ":*")
					}
					if m.session.PermSvc().AutoMode() != nil {
						m.session.PermSvc().AutoMode().Record(toolName, summary, true)
					}
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CheckBold() + " Always allowed: " + toolName + " (all)"})
			case "d", "D":
				req := m.permReq
				toolName := req.ToolName
				summary := req.Summary
				req.Response <- false
				m.permReq = nil
				m.permTimeoutAt = time.Time{}
				if m.session != nil && m.session.PermSvc() != nil {
					if mem := m.session.PermSvc().Memory(); mem != nil {
						mem.AlwaysDeny(toolName)
					}
					if m.session.PermSvc().AutoMode() != nil {
						m.session.PermSvc().AutoMode().Record(toolName, summary, false)
					}
				}
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CloseThick() + " Always denied: " + toolName})
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		}

		// Credential prompt active — handle y/n
		if m.credentialReq != nil {
			switch msg.String() {
			case "y", "Y":
				req := m.credentialReq
				req.response <- tool.CredentialResponse{Approved: true}
				m.credentialReq = nil
				m.credentialTimeoutAt = time.Time{}
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CheckBold() + " Credential access granted: " + req.req.Name})
			case "n", "N":
				req := m.credentialReq
				req.response <- tool.CredentialResponse{Approved: false, Reason: "denied by user"}
				m.credentialReq = nil
				m.credentialTimeoutAt = time.Time{}
				m.messages = append(m.messages, displayMsg{role: "system", content: icons.CloseThick() + " Credential access denied: " + req.req.Name})
			}
			m.viewDirty = true
			m.updateViewportContent()
			return m, nil
		}

		// Container failed and is retryable. Hawk is fail-closed: the only
		// recovery path is to restore Docker isolation.
		if m.containerRetryable {
			switch msg.String() {
			case "r", "R":
				m.containerRetryable = false
				m.containerEnabled = true
				m.containerErr = nil
				m.containerStatus = "checking docker…"
				if m.session != nil {
					m.session.SetContainerRequired(true)
				}
				// Silent retry — no chat message spam. The welcome badge
				// already reflects container state, and a failure below
				// will surface only if the banner is no longer visible.
				m.viewDirty = true
				m.updateViewportContent()
				cwd, _ := os.Getwd()
				return m, bootContainerCmd(cwd)
			}
		}
		// AskUser prompt active — Enter submits answer
		if m.askReq != nil {
			if msg.String() == "enter" {
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
			if msg.String() == "ctrl+c" {
				// First Ctrl+C cancels stream, second quits
				if m.cancel != nil {
					m.lastCtrlC = time.Now()
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
				return m.quitModel()
			}
			if msg.String() == "escape" {
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
			if msg.String() == "enter" {
				text := strings.TrimSpace(m.input.Value())
				if text != "" {
					m.pushHistory(text)
					m.enqueueMessage(text)
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
			switch msg.String() {
			case "ctrl+c":
				if time.Since(m.lastCtrlC) < 1*time.Second {
					return m.quitModel()
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

		// Handle modifier key combos (ctrl+a, ctrl+k, etc.) via string matching.
		switch msg.Key().Mod {
		case 0: // no modifier
		default:
			// modifier combos: check the keystroke string
			keyText := msg.String()
			if keyText != "" {
				switch keyText {
				case "ctrl+a":
					m.hudOpen = !m.hudOpen
					if m.hudOpen {
						m.hudData = m.collectHUDData()
					}
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				case "ctrl+k", "ctrl+p":
					if m.commandPalette == nil {
						m.commandPalette = NewCommandPalette(m.width)
					}
					m.commandPalette.Open()
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				case "ctrl+r":
					// Reverse-i-search: only when input is empty and not waiting.
					if strings.TrimSpace(m.input.Value()) == "" && !m.waiting {
						m.historySearchOpen = true
						m.historySearchInput = ""
						m.historySearchQuery = ""
						m.historySearchFiltered = nil
						m.historySearchSel = 0
						m.applyHistorySearchFilter()
						m.viewDirty = true
						m.updateViewportContent()
					}
					return m, nil
				case "ctrl+s":
					// Session picker: fuzzy search through saved sessions.
					if !m.waiting {
						m.sessionPickerOpen = true
						m.sessionPickerInput = ""
						m.sessionPickerEntries, _ = session.List()
						m.sessionPickerFiltered = m.sessionPickerEntries
						m.sessionPickerSel = 0
						m.viewDirty = true
						m.updateViewportContent()
					}
					return m, nil
				case "?":
					// Quick help — show contextual help summary in chat.
					m.messages = append(m.messages, displayMsg{role: "system", content: "Quick help:\n  /start           — guided setup (trust, mode, branch)\n  /mode plan|act   — research vs build\n  /isolation       — sandbox profile\n  /help            — list all commands\n  /help <topic>    — detailed help (e.g., /help /commit)\n  ctrl+K           — command palette\n  ctrl+L           — cycle autonomy tiers\n  ctrl+N           — switch model\n  ctrl+R           — search input history\n  ?                — show this help\n  Type / to see slash commands, or ask a question to get started."})
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				case "ctrl+n":
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
				case "ctrl+l":
					if m.containerEnabled && !m.containerReady {
						m.messages = append(m.messages, displayMsg{role: "system", content: "Waiting for container — higher tiers unlock when the Docker container is ready."})
						m.viewDirty = true
						m.updateViewportContent()
						return m, nil
					}
					// Expire a stale Supervised-confirmation prompt.
					if m.supervisedPending && time.Since(m.supervisedPendingAt) > 1500*time.Millisecond {
						m.supervisedPending = false
					}
					current := m.session.PermSvc().Autonomy()
					// Guard landing on Supervised: when the cycle would reach it
					// (current is YOLO), require a second Ctrl+L within 1.5s. This
					// prevents accidental max-friction while keeping it one deliberate
					// gesture away.
					if isSupervisedPending(current) && !m.supervisedPending {
						m.supervisedPending = true
						m.supervisedPendingAt = time.Now()
						m.messages = append(m.messages, displayMsg{
							role:    "warning",
							content: "Ctrl+L again within 1.5s to confirm Always Ask (max friction), or wait to skip.",
						})
						m.viewDirty = true
						m.updateViewportContent()
						return m, nil
					}
					var nextTier engine.AutonomyLevel
					if m.supervisedPending && isSupervisedPending(current) {
						nextTier = nextAutonomyTierIncludingSupervised(current)
					} else {
						nextTier = nextAutonomyTier(current)
					}
					if current == 0 || autonomyTierIndex(current) < 0 {
						nextTier = DefaultContainerAutonomy
					}
					m.supervisedPending = false
					m.session.PermSvc().SetAutonomy(nextTier)
					m.settings.AutonomyExplicit = true
					m.invalidateConnStatus()
					m.messages = append(m.messages, displayMsg{
						role:    "warning",
						content: formatAutonomyTierMessage(nextTier) + "  ·  Ctrl+L to change",
					})
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				case "ctrl+c":
					if time.Since(m.lastCtrlC) < 1*time.Second {
						return m.quitModel()
					}
					m.lastCtrlC = time.Now()
					m.messages = append(m.messages, displayMsg{role: "system", content: quitAgainMsg})
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				case "shift+tab":
					if m.specPicker == nil {
						m.specPicker = NewSpecPicker(m.width)
					}
					m.specPicker.Open(currentSpecStage(m.session))
					m.viewDirty = true
					m.updateViewportContent()
					return m, nil
				}
			}
		}

		// Main key dispatch (special keys via code, runes via text)
		switch msg.Key().Code {
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
			if m.inScrollbackFocus() {
				return m.cycleUIFocus()
			}
			if m.waiting {
				return m, nil
			}
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

	case voiceResultMsg:
		// The /voice subcommand records and transcribes on a background
		// goroutine and reports back here, so all model mutation stays on the
		// Bubble Tea goroutine (no data race on m.messages / m.input).
		switch {
		case msg.err != "":
			m.messages = append(m.messages, displayMsg{role: "error", content: msg.err})
		case msg.info != "":
			m.messages = append(m.messages, displayMsg{role: "system", content: msg.info})
		case msg.transcript != "":
			m.input.SetValue(msg.transcript)
			m.input.CursorEnd()
			m.messages = append(m.messages, displayMsg{role: "system", content: fmt.Sprintf("Voice input: %s", msg.transcript)})
		}
		m.viewDirty = true
		m.updateViewportContent()
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
		// Durability: persist completed tool results incrementally so a
		// crash mid-turn doesn't lose them (they were previously only
		// written at turn end via saveSession).
		m.ensureWAL()
		if m.wal != nil {
			m.walSeq++
			m.recordWALError(m.wal.Append(session.Message{Role: "tool_result", Content: msg.content}))
		}

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
		m.permTimeoutAt = time.Now().Add(5 * time.Minute)
		// Display-only enrichment (risk + why). Keep req.Summary as ToolSummary
		// for AutoMode / memory matching after y/n/a/d.
		permBody := engine.FormatPermissionDisplay(msg.req.ToolName, msg.req.Summary)
		m.messages = append(m.messages, displayMsg{role: "permission", content: permBody, timeoutAt: m.permTimeoutAt})
		m.viewDirty = true
		m.updateViewportContent()
		return m, permissionPromptTimeoutCmd(m.permReqSeq)

	case permissionPromptTimeoutMsg:
		if m.permReq != nil && m.permReqSeq == msg.seq {
			// Send denial response to unblock the waiting goroutine.
			m.permReq.Response <- false
			m.permReq = nil
			m.permTimeoutAt = time.Time{}
			m.messages = append(m.messages, displayMsg{role: "system", content: icons.Timer() + " Permission prompt timed out — denied."})
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
			// Send empty response to unblock the waiting goroutine.
			m.askReq.response <- ""
			m.askReq = nil
			m.messages = append(m.messages, displayMsg{role: "system", content: icons.Timer() + " Question timed out."})
			m.viewDirty = true
			m.updateViewportContent()
			return m, m.input.Focus()
		}
		return m, nil

	case credentialAskMsg:
		m.credentialReq = &msg
		m.credentialReqSeq++
		m.credentialTimeoutAt = time.Now().Add(5 * time.Minute)
		prompt := fmt.Sprintf("AI wants to access %s (%s): %s",
			msg.req.Name, msg.req.Credential, msg.req.Reason)
		m.messages = append(m.messages, displayMsg{role: "credential", content: prompt, timeoutAt: m.credentialTimeoutAt})
		m.viewDirty = true
		m.updateViewportContent()
		return m, credentialPromptTimeoutCmd(m.credentialReqSeq)

	case credentialPromptTimeoutMsg:
		if m.credentialReq != nil && m.credentialReqSeq == msg.seq {
			m.credentialReq.response <- tool.CredentialResponse{Approved: false, Reason: "timed out"}
			m.credentialReq = nil
			m.credentialTimeoutAt = time.Time{}
			m.messages = append(m.messages, displayMsg{role: "system", content: icons.Timer() + " Credential request timed out — denied."})
			m.viewDirty = true
			m.updateViewportContent()
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
			m.ensureWAL()
			if m.wal != nil {
				m.walSeq++
				m.recordWALError(m.wal.Append(session.Message{Role: "assistant", Content: content}))
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
		// Invalidate slash suggestion cache — new messages may have changed
		// the available command set (e.g. plugin-registered commands).
		m.invalidateSlashSugCache()
		// Save flags before reset so the notification check below sees
		// the values from the turn that just completed.
		hadOutput := m.turnHadAssistantOutput
		wasCancelled := m.streamCancelled
		m.turnHadAssistantOutput = false
		m.turnHadToolActivity = false
		// Resolve any pending permission/askUser prompts to unblock waiting goroutines.
		if m.permReq != nil {
			m.permReq.Response <- false
			m.permReq = nil
		}
		if m.askReq != nil {
			m.askReq.response <- ""
			m.askReq = nil
		}
		m.waiting = false
		m.cancel = nil
		m.toolStartTime = time.Time{}
		m.viewDirty = true
		m.input.Focus()
		// Persist off the UI thread: a large session JSONL write would
		// otherwise hitch the completion frame. The WAL is removed only after
		// the save succeeds (see saveSessionCmd).
		if saveCmd := m.saveSessionCmd(); saveCmd != nil {
			cmds = append(cmds, saveCmd)
		}

		// Trim old messages to prevent unbounded memory growth in long sessions.
		m.trimOldMessages()

		// Re-enable system sleep now that the turn is complete.
		if m.sleepCancel != nil {
			m.sleepCancel()
			m.sleepCancel = nil
		}
		// Clear the terminal tab progress bar now that the turn is done.
		ClearTabProgress()

		// Send terminal notification if terminal was not focused during the
		// turn and the agent produced output (not just tool activity).
		if m.backgrounded && !wasCancelled && hadOutput {
			sendTerminalNotification("hawk", "Agent turn complete")
		}
		m.backgrounded = false
		m.notifiedComplete = false

		// Process queued messages
		if len(m.messageQueue) > 0 {
			nextMsg := m.messageQueue[0]
			m.messageQueue = m.messageQueue[1:]
			m.messages = append(m.messages, displayMsg{role: "user", content: nextMsg})
			m.session.AddUser(nextMsg)
			m.waiting = true
			m.autoScroll = true
			m.viewDirty = true
			m.spinnerVerb = spinnerVerbs[rand.IntN(len(spinnerVerbs))] // #nosec G404 -- non-cryptographic use (random spinner verb selection)
			m.brailleSpinner.SetLabel(m.spinnerVerb)
			m.turnSawThinking = false
			m.turnHadAssistantOutput = false
			m.turnHadToolActivity = false
			m.turnInputTokens = 0
			m.turnOutputTokens = 0
			m.turnEstimatedOutputRunes = 0
			m.startedAt = time.Now()
			m.partial.Reset()
			m.startStream()
			return m, tea.Batch(m.spinner.Tick, spinnerVerbTickCmd())
		}

	case streamErrMsg:
		m.messages = append(m.messages, displayMsg{role: "error", content: friendlyError(msg.err)})
		m.partial.Reset()
		// Resolve any pending permission/askUser prompts to unblock waiting goroutines.
		if m.permReq != nil {
			m.permReq.Response <- false
			m.permReq = nil
		}
		if m.askReq != nil {
			m.askReq.response <- ""
			m.askReq = nil
		}
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
			m.spinnerVerb = spinnerVerbs[rand.IntN(len(spinnerVerbs))] // #nosec G404 -- non-cryptographic use (random spinner verb selection)
			m.brailleSpinner.SetLabel(m.spinnerVerb)
			m.viewDirty = true
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(msg.Width - 4)
		m.configInput.SetWidth(msg.Width - 4)
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
			m.displayInTok += (float64(m.tokenInputTarget()) - m.displayInTok) * 0.25
			m.displayOutTok += (float64(m.tokenOutputTarget()) - m.displayOutTok) * 0.25
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
				m.session.ApplyIsolationProfile(engine.IsolationContainer)
				m.session.SetContainerExecutor(msg.sandbox)
			}
		}
		if msg.ready && m.session != nil {
			m.session.ApplyIsolationProfile(engine.IsolationContainer)
			if m.session.PermSvc().Autonomy() == 0 && !m.session.PermSvc().AutonomyExplicit() {
				m.session.PermSvc().SetAutonomy(DefaultContainerAutonomy)
			}
			m.invalidateConnStatus()
			m.containerRetryable = false
			// Auto-submit any input that was queued while the container was booting.
			if m.pendingSubmit != "" {
				m.input.SetValue(m.pendingSubmit)
				m.pendingSubmit = ""
				m.rebuildWelcomeCache(m.blinkClosed)
				m.viewDirty = true
				m.updateViewportContent()
				return m.submitUserMessage()
			}
		}
		if msg.err != nil {
			// Docker-only execution fails closed. Keep the session container
			// requirement enabled and disable every tool until retry succeeds.
			m.containerEnabled = true
			m.containerReady = false
			m.containerRetryable = true
			if m.session != nil {
				m.session.SetContainerRequired(true)
				m.session.SetContainerExecutor(nil)
			}
			m.messages = append(m.messages, displayMsg{
				role: "warning",
				content: icons.Alert() + " Docker isolation required\n" +
					msg.err.Error() + "\n" +
					icons.Refresh() + " Press r to retry  ·  Ctrl+C to quit",
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
					m.input.SetCursorColumn(newCursor)
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
