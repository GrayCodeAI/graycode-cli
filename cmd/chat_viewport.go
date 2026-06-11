package cmd

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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

// syncViewportMouseWheel keeps viewport wheel handling on whenever chat is shown.
// Zone routing in shouldRouteMouseToViewport decides input vs chat targets.
func (m chatModel) syncViewportMouseWheel() chatModel {
	m.viewport.MouseWheelEnabled = !m.configOpen && !m.onWelcomeGate()
	return m
}

// shouldRouteMouseToViewport decides whether mouse events reach the chat viewport.
// Standard split-pane UX: wheel over chat scrolls history; wheel over input is ignored;
// arrows in prompt focus navigate input history (see routeKeyToViewport).
func (m chatModel) shouldRouteMouseToViewport(msg tea.Msg) bool {
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
