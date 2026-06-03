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