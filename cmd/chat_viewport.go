package cmd

import (
	"regexp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// mouseMsg wraps a tea.Mouse to implement tea.MouseMsg interface for tests.
type mouseMsg struct {
	mouse tea.Mouse
}

func (m mouseMsg) String() string   { return m.mouse.String() }
func (m mouseMsg) Mouse() tea.Mouse { return m.mouse }

// mouseSGRLeakRE matches SGR mouse reports; "[" is optional (Cursor often drops it).
var mouseSGRLeakRE = regexp.MustCompile(`(?:\x1b)?\[?<[0-9;.+^$*-]+[Mm]`)

// mouseSGRLeakPartialRE matches CSI mouse bytes split across KeyRunes events.
// Must include "[" or "<" + digit — never a bare letter (typing "m" must pass through).
var mouseSGRLeakPartialRE = regexp.MustCompile(`^(?:\x1b\[|\[)(?:<[0-9;.+^$*-]*)?[Mm]?$|^<[0-9][0-9;.+^$*-]*[Mm]?$`)

// mouseSGRLeakTailRE matches tails like "<64;84;24M" or "65;84;24M".
var mouseSGRLeakTailRE = regexp.MustCompile(`^<?[0-9]+(?:;[0-9]+)+[Mm]$`)

// mouseSGRStripRE removes complete and partial SGR mouse garbage from input text.
// Requires "<" + digit or a bracket-led CSI prefix so words ending in "m" are kept.
var mouseSGRStripRE = regexp.MustCompile(`(?:\x1b)?\[?<[0-9][0-9;.+^$*-]*[Mm]?|(?:\x1b)?\[(?:<[0-9;.+^$*-]*)?[Mm]?|<?[0-9]+(?:;[0-9]+)+[Mm]`)

// mouseSGRReportRE parses xterm SGR mouse reports (e.g. "[<65;99;16M" or "<65;99;16M").
var mouseSGRReportRE = regexp.MustCompile(`(?:\x1b)?\[?<(\d+);(\d+);(\d+)([Mm])`)

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
	if len(msg.Key().Text) == 0 {
		return false
	}
	return isMouseRuneLeak(msg.Key().Text)
}

// stripMouseLeaks removes accumulated SGR mouse garbage from an input value.
func stripMouseLeaks(s string) string {
	if s == "" || (!strings.Contains(s, "<") && !strings.Contains(s, "\x1b")) {
		return s
	}
	for {
		next := mouseSGRStripRE.ReplaceAllString(s, "")
		if next == s {
			return next
		}
		s = next
	}
}

func inputMayContainMouseLeaks(s string) bool {
	if s == "" {
		return false
	}
	return strings.Contains(s, "<") || strings.Contains(s, "\x1b")
}

// isMouseWheel returns true if the mouse message represents a scroll (wheel) event.
func isMouseWheel(msg tea.MouseMsg) bool {
	m := msg.Mouse()
	return m.Button == tea.MouseWheelUp || m.Button == tea.MouseWheelDown
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
	if m.arrowBurstActive {
		// Only Up/Down are part of the burst this flag describes. Any other
		// key (typing, Escape, ...) must fall through to the normal routing
		// below rather than being swept into viewport-scroll handling.
		switch msg.Key().Code {
		case tea.KeyUp, tea.KeyDown:
			if m.lastMouseY >= 0 {
				return m.mouseInChatPane(mouseMsg{tea.Mouse{Y: m.lastMouseY}})
			}
			return true
		}
		return false
	}
	if m.configOpen {
		return false
	}
	s := msg.String()
	if m.inScrollbackFocus() {
		switch s {
		case "pgup", "pgdown", "ctrl+u", "ctrl+d", "u", "d", "f", "b", "k", "j", " ":
			return m.viewportScrollable()
		}
		return false
	}
	if !m.viewportScrollable() {
		return false
	}
	switch s {
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		return true
	case "up", "k", "u", "b", "down", "j", "d", "f":
		// Prompt focus: these are plain typeable letters (u/d/f/b) as well as
		// Up/Down — they must reach the input as text/history nav, not be
		// hijacked as vim-style pager scroll keys (Charm chat-input pattern).
		if m.uiFocus == focusPrompt {
			return false
		}
		if strings.TrimSpace(m.input.Value()) != "" {
			return false
		}
		switch s {
		case "up", "k", "u", "b":
			return !m.viewport.AtTop()
		default:
			return !m.viewport.AtBottom()
		}
	case " ":
		return strings.TrimSpace(m.input.Value()) == ""
	}
	return false
}

// chatPaneTopY is the first terminal row of the scrollable chat pane (sync with View).
func (m chatModel) chatPaneTopY() int {
	if m.height <= 0 {
		return 0
	}
	m = m.withSyncedLayout()
	top := m.footerTopY() - m.viewport.Height()
	if top < 0 {
		top = 0
	}
	return top
}

// footerTopY is the first terminal row of the fixed footer (input + stats), exclusive
// upper bound for the scrollable chat pane. Keep in sync with View().
func (m chatModel) footerTopY() int {
	if m.height <= 0 {
		return 0
	}
	m = m.withSyncedLayout()
	return m.height - m.chatBottomBarLines()
}

// bottomBarTopY is the first terminal row of the fixed footer (alias for mouse routing).
func (m chatModel) bottomBarTopY() int {
	return m.footerTopY()
}

// mouseInFooterZone reports whether a mouse event is over the fixed footer (input + stats).
func (m chatModel) mouseInFooterZone(mouse tea.MouseMsg) bool {
	if m.height <= 0 {
		return false
	}
	m = m.withSyncedLayout()
	return mouse.Mouse().Y >= m.footerTopY()
}

// mouseInChatPane reports whether a mouse event is over the chat viewport region.
func (m chatModel) mouseInChatPane(mouse tea.MouseMsg) bool {
	if m.height <= 0 {
		return true
	}
	m = m.withSyncedLayout()
	mouseEv := mouse.Mouse()
	top := m.chatPaneTopY()
	footerTop := m.footerTopY()
	if footerTop <= top {
		return mouseEv.Y >= top
	}
	return mouseEv.Y >= top && mouseEv.Y < footerTop
}

// trackMousePosition remembers the last pointer row for wheel routing.
func (m *chatModel) trackMousePosition(msg tea.MouseMsg) {
	mouse := msg.Mouse()
	if mouse.Y < 0 {
		return
	}
	// Cursor wheel leaks often report the footer row; keep the last motion/chat row instead.
	if (mouse.Button == tea.MouseWheelUp || mouse.Button == tea.MouseWheelDown) && !m.mouseInChatPane(msg) {
		return
	}
	m.lastMouseY = mouse.Y
}

// effectiveWheelY picks the row used to route wheel events. Cursor's integrated terminal
// often reports wheel at the bottom row even when the pointer is over chat; prefer the
// last known pointer row only for that stale bottom-row report.
func (m chatModel) effectiveWheelY(msg tea.MouseMsg) int {
	mouse := msg.Mouse()
	y := mouse.Y
	if m.lastMouseY < 0 || !m.mouseInFooterZone(msg) || m.height <= 0 {
		return y
	}
	if y < m.height-1 {
		return y
	}
	if m.mouseInChatPane(mouseMsg{tea.Mouse{Y: m.lastMouseY}}) {
		return m.lastMouseY
	}
	return y
}

// syncViewportMouseWheel disables bubbletea viewport auto-wheel; hawk routes wheel
// events manually so chat scrolls only when the pointer is over the chat pane.
func (m chatModel) syncViewportMouseWheel() chatModel {
	m.viewport.MouseWheelEnabled = false
	return m
}

// shouldRouteMouseToViewport decides whether mouse events reach the chat viewport.
// Standard split-pane UX: wheel over chat scrolls history; wheel over input is ignored;
// arrows in prompt focus navigate input history (see routeKeyToViewport).
func (m chatModel) shouldRouteMouseToViewport(msg tea.Msg) bool {
	if !m.mouseEnabled() {
		return false
	}
	mouse, isMouse := msg.(tea.MouseMsg)
	if !isMouse {
		return true
	}
	if !isMouseWheel(mouse) {
		return m.inScrollbackFocus()
	}
	if m.configOpen {
		return false
	}
	if m.inScrollbackFocus() {
		return true
	}
	return m.wheelRoutesToChat(mouse)
}

// wheelRoutesToChat reports whether a wheel event should scroll chat history.
func (m chatModel) wheelRoutesToChat(mouse tea.MouseMsg) bool {
	route := mouse.Mouse()
	route.Y = m.effectiveWheelY(mouse)
	return m.mouseInChatPane(mouseMsg{route})
}

// applyMouseScroll routes a mouse event to the chat viewport and syncs follow mode.
func (m *chatModel) applyMouseScroll(msg tea.MouseMsg) tea.Cmd {
	if !isMouseWheel(msg) {
		if !m.shouldRouteMouseToViewport(msg) {
			return nil
		}
	} else if !m.wheelRoutesToChat(msg) {
		return nil
	}
	switch msg.Mouse().Button {
	case tea.MouseWheelDown:
		m.viewport.ScrollDown(m.viewport.MouseWheelDelta)
	case tea.MouseWheelUp:
		m.viewport.ScrollUp(m.viewport.MouseWheelDelta)
	default:
		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		if vpCmd != nil {
			return vpCmd
		}
	}
	if m.viewport.AtBottom() {
		m.autoScroll = true
		if m.uiFocus == focusPrompt {
			m.streamFollow = true
		}
	} else {
		m.autoScroll = false
		m.streamFollow = false
	}
	return nil
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
		return tea.MouseWheelUp, true
	case 65:
		return tea.MouseWheelDown, true
	default:
		return 0, false
	}
}

func mouseMsgFromSGRMatch(match []string) (tea.MouseMsg, bool) {
	if len(match) < 5 {
		return tea.MouseClickMsg{}, false
	}
	btnCode, err1 := strconv.Atoi(match[1])
	x, err2 := strconv.Atoi(match[2])
	y, err3 := strconv.Atoi(match[3])
	if err1 != nil || err2 != nil || err3 != nil {
		return tea.MouseClickMsg{}, false
	}
	// SGR coordinates are 1-based; bubbletea uses 0-based (see parseSGRMouseEvent).
	if x > 0 {
		x--
	}
	if y > 0 {
		y--
	}
	btn, ok := wheelButtonFromSGR(btnCode)
	if !ok {
		return tea.MouseClickMsg{}, false
	}
	return tea.MouseClickMsg{
		X:      x,
		Y:      y,
		Button: btn,
	}, true
}

// tryScrollFromMouseLeak handles terminals (e.g. Cursor) that inject wheel events as
// literal "[<65;x;yM" / "<65;x;yM" KeyRunes instead of tea.MouseMsg. Routes by Y:
// chat scrolls, input/footer is ignored.
func (m *chatModel) tryScrollFromMouseLeak(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.mouseEnabled() {
		return false, nil
	}
	matches := mouseSGRReportRE.FindAllStringSubmatch(msg.Key().Text, -1)
	if len(matches) == 0 {
		return false, nil
	}
	var cmd tea.Cmd
	for _, match := range matches {
		mouse, ok := mouseMsgFromSGRMatch(match)
		if !ok {
			continue
		}
		m.trackMousePosition(mouse)
		if m.wheelRoutesToChat(mouse) {
			cmd = m.applyMouseScroll(mouse)
		}
	}
	return true, cmd
}

func (m *chatModel) ensurePromptInputFocus() tea.Cmd {
	if m.uiFocus == focusPrompt && !m.configOpen && !m.waiting && !m.useConfigInput {
		return m.input.Focus()
	}
	return nil
}

func (m *chatModel) sanitizeInputIfNeeded() {
	val := m.input.Value()
	if !inputMayContainMouseLeaks(val) {
		return
	}
	cleaned := stripMouseLeaks(val)
	if cleaned != val {
		m.input.SetValue(cleaned)
		m.input.CursorEnd()
	}
}

// updateInput forwards a message to the textarea when it is safe (not mouse noise).
func (m *chatModel) updateInput(msg tea.Msg) tea.Cmd {
	if !shouldForwardToInput(msg) {
		m.sanitizeInputIfNeeded()
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if inputMayContainMouseLeaks(m.input.Value()) {
		m.sanitizeInputIfNeeded()
	}
	return cmd
}
