package cmd

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// mouseSGRLeakRE matches SGR mouse reports; "[" is optional (Cursor often drops it).
var mouseSGRLeakRE = regexp.MustCompile(`(?:\x1b)?\[?<[0-9;.+^$*-]+[Mm]`)

// mouseSGRLeakPartialRE matches CSI mouse bytes split across KeyRunes events.
var mouseSGRLeakPartialRE = regexp.MustCompile(`^[\[<]?<?[0-9;.+^$*-]*[Mm]?$`)

// mouseSGRLeakTailRE matches tails like "<64;84;24M" or "65;84;24M".
var mouseSGRLeakTailRE = regexp.MustCompile(`^<?[0-9]+(?:;[0-9]+)*[Mm]$`)

// mouseSGRStripRE removes complete and partial SGR mouse garbage from input text.
var mouseSGRStripRE = regexp.MustCompile(`(?:\x1b)?\[?<[0-9;.+^$*-]*[Mm]?`)

// mouseSGRReportRE parses xterm SGR mouse reports (e.g. "[<65;99;16M" or "<65;99;16M").
var mouseSGRReportRE = regexp.MustCompile(`(?:\x1b)?\[?<(\d+);(\d+);(\d+)([Mm])`)

// mouseTrackingEnabled is on by default for split-pane wheel scroll (chat yes, input no).
// Set HAWK_MOUSE=0 to disable wheel scrolling entirely (broken terminals).
func mouseTrackingEnabled() bool {
	v := strings.TrimSpace(os.Getenv("HAWK_MOUSE"))
	if v == "0" || strings.EqualFold(v, "false") || strings.EqualFold(v, "off") {
		return false
	}
	return true
}

func isMouseRuneLeak(s string) bool {
	if s == "" {
		return false
	}
	if mouseSGRReportRE.MatchString(s) || mouseSGRLeakRE.MatchString(s) {
		return true
	}
	if mouseSGRLeakPartialRE.MatchString(s) {
		return true
	}
	if mouseSGRLeakTailRE.MatchString(s) {
		return true
	}
	return false
}

// isMouseSequenceLeak reports terminal mouse CSI fragments that must not reach the input.
func isMouseSequenceLeak(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return false
	}
	return isMouseRuneLeak(string(msg.Runes))
}

// stripMouseLeaks removes accumulated SGR mouse garbage from an input value.
func stripMouseLeaks(s string) string {
	for {
		next := mouseSGRStripRE.ReplaceAllString(s, "")
		if next == s {
			return next
		}
		s = next
	}
}

// shouldForwardToInput keeps mouse events and leaked CSI bytes out of the textarea.
func shouldForwardToInput(msg tea.Msg) bool {
	if _, ok := msg.(tea.MouseMsg); ok {
		return false
	}
	if key, ok := msg.(tea.KeyMsg); ok && isMouseSequenceLeak(key) {
		return false
	}
	return true
}

// viewportScrollable reports whether chat content extends beyond the viewport.
func (m chatModel) viewportScrollable() bool {
	return !(m.viewport.AtTop() && m.viewport.AtBottom())
}

// routeKeyToViewport returns true when the key should scroll chat history instead of the input.
func (m chatModel) routeKeyToViewport(msg tea.KeyMsg) bool {
	if m.configOpen {
		return false
	}
	s := msg.String()
	if m.inScrollbackFocus() {
		switch s {
		case "pgup", "pgdown", "ctrl+u", "ctrl+d", "u", "d", "f", "b", "up", "k", "down", "j", " ":
			return m.viewportScrollable()
		}
		return false
	}
	if !m.viewportScrollable() {
		return false
	}
	switch s {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d", "u", "d", "f", "b":
		return true
	case "up", "k", "down", "j":
		// Prompt focus: Up/Down always drive input history (Charm chat-input pattern).
		if m.uiFocus == focusPrompt {
			return false
		}
		if strings.TrimSpace(m.input.Value()) != "" {
			return false
		}
		if s == "up" || s == "k" {
			return !m.viewport.AtTop()
		}
		return !m.viewport.AtBottom()
	case " ":
		return strings.TrimSpace(m.input.Value()) == ""
	}
	return false
}

// chatPaneTopY is the first terminal row of the scrollable chat pane (sync with View).
func (m chatModel) chatPaneTopY() int {
	top := m.fixedWelcomeLineCount()
	if top > 0 {
		top++
	}
	return top
}

// bottomBarTopY is the first terminal row of the fixed footer (input + stats).
func (m chatModel) bottomBarTopY() int {
	if m.height <= 0 {
		return 0
	}
	return m.height - m.chatBottomBarLines()
}

// mouseInChatPane reports whether a mouse event is over the chat viewport region.
func (m chatModel) mouseInChatPane(mouse tea.MouseMsg) bool {
	if m.height <= 0 {
		return true
	}
	top := m.chatPaneTopY()
	bottom := m.bottomBarTopY()
	if bottom <= top {
		return mouse.Y >= top
	}
	return mouse.Y >= top && mouse.Y < bottom
}

// syncViewportMouseWheel enables wheel scrolling only when mouse tracking is on.
func (m chatModel) syncViewportMouseWheel() chatModel {
	m.viewport.MouseWheelEnabled = mouseTrackingEnabled() && !m.configOpen && !m.onWelcomeGate()
	return m
}

// shouldRouteMouseToViewport decides whether mouse events reach the chat viewport.
// Standard split-pane UX: wheel over chat scrolls history; wheel over input is ignored;
// arrows in prompt focus navigate input history (see routeKeyToViewport).
func (m chatModel) shouldRouteMouseToViewport(msg tea.Msg) bool {
	if !mouseTrackingEnabled() {
		return false
	}
	mouse, isMouse := msg.(tea.MouseMsg)
	if !isMouse {
		return true
	}
	if !tea.MouseEvent(mouse).IsWheel() {
		return m.inScrollbackFocus()
	}
	if m.configOpen || m.onWelcomeGate() {
		return false
	}
	if !m.viewportScrollable() {
		return false
	}
	if m.inScrollbackFocus() {
		return true
	}
	return m.mouseInChatPane(mouse)
}

// applyViewportScroll updates the chat viewport and syncs auto-scroll with scroll position.
// The bool is true when the key was handled as a scroll action (even if cmd is nil).
func (m *chatModel) applyViewportScroll(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.routeKeyToViewport(msg) {
		return false, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	if m.viewport.AtBottom() {
		m.autoScroll = true
		if m.uiFocus == focusPrompt {
			m.streamFollow = true
		}
	} else {
		m.autoScroll = false
		m.streamFollow = false
	}
	return true, cmd
}

func wheelButtonFromSGR(code int) (tea.MouseButton, bool) {
	switch code {
	case 64:
		return tea.MouseButtonWheelUp, true
	case 65:
		return tea.MouseButtonWheelDown, true
	default:
		return 0, false
	}
}

func mouseMsgFromSGRMatch(match []string) (tea.MouseMsg, bool) {
	if len(match) < 5 {
		return tea.MouseMsg{}, false
	}
	btnCode, err1 := strconv.Atoi(match[1])
	x, err2 := strconv.Atoi(match[2])
	y, err3 := strconv.Atoi(match[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return tea.MouseMsg{}, false
	}
	btn, ok := wheelButtonFromSGR(btnCode)
	if !ok {
		return tea.MouseMsg{}, false
	}
	return tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: btn,
		Action: tea.MouseActionPress,
	}, true
}

// tryScrollFromMouseLeak handles terminals (e.g. Cursor) that inject wheel events as
// literal "[<65;x;yM" / "<65;x;yM" KeyRunes instead of tea.MouseMsg. Routes by Y:
// chat scrolls, input/footer is ignored.
func (m *chatModel) tryScrollFromMouseLeak(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !mouseTrackingEnabled() {
		return false, nil
	}
	matches := mouseSGRReportRE.FindAllStringSubmatch(string(msg.Runes), -1)
	if len(matches) == 0 {
		return false, nil
	}
	var cmd tea.Cmd
	for _, match := range matches {
		mouse, ok := mouseMsgFromSGRMatch(match)
		if !ok {
			continue
		}
		if m.shouldRouteMouseToViewport(mouse) {
			cmd = m.applyMouseScroll(mouse)
		}
	}
	return true, cmd
}

// applyMouseScroll routes a mouse event to the chat viewport and syncs follow mode.
func (m *chatModel) applyMouseScroll(msg tea.MouseMsg) tea.Cmd {
	if !m.shouldRouteMouseToViewport(msg) {
		return nil
	}
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	if m.viewport.AtBottom() {
		m.autoScroll = true
		if m.uiFocus == focusPrompt {
			m.streamFollow = true
		}
	} else {
		m.autoScroll = false
		if m.uiFocus == focusScrollback {
			m.streamFollow = false
		}
	}
	return vpCmd
}

// sanitizeInput strips any SGR mouse garbage already present in the textarea.
func (m *chatModel) sanitizeInput() {
	cleaned := stripMouseLeaks(m.input.Value())
	if cleaned != m.input.Value() {
		m.input.SetValue(cleaned)
		m.input.CursorEnd()
	}
}

// updateInput forwards a message to the textarea when it is safe (not mouse noise).
func (m *chatModel) updateInput(msg tea.Msg) tea.Cmd {
	if !shouldForwardToInput(msg) {
		m.sanitizeInput()
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.sanitizeInput()
	return cmd
}
