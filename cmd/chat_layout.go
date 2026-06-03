package cmd

import (
	"strings"
)

const minChatViewportLines = 4

// fixedWelcomeLineCount reserves room for the branded welcome pane above chat.
func (m chatModel) fixedWelcomeLineCount() int {
	if strings.TrimSpace(m.welcomeCache) == "" || m.onWelcomeGate() {
		return 0
	}
	lines := strings.Split(strings.TrimRight(m.welcomeCache, "\n"), "\n")
	count := len(lines)
	if m.height <= 0 {
		return count
	}
	max := m.height - m.chatBottomBarLines() - minChatViewportLines - 1
	if max < 0 {
		max = 0
	}
	if count > max {
		count = max
	}
	return count
}

// renderFixedWelcomePane draws the welcome screen above the scrollable chat viewport.
func (m chatModel) renderFixedWelcomePane(width int) string {
	if m.fixedWelcomeLineCount() == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(m.welcomeCache, "\n"), "\n")
	max := m.fixedWelcomeLineCount()
	if len(lines) > max {
		lines = lines[:max]
	}
	return strings.Join(lines, "\n")
}

// withSyncedLayout returns m with viewport size reserved for welcome + bottom chrome.
func (m chatModel) withSyncedLayout() chatModel {
	if m.height <= 0 {
		return m
	}
	bottomH := m.chatBottomBarLines()
	welcomeH := m.fixedWelcomeLineCount()
	vpH := m.height - bottomH - welcomeH
	if welcomeH > 0 {
		vpH--
	}
	if m.onWelcomeGate() {
		vpH = minChatViewportLines
	}
	if vpH < minChatViewportLines {
		vpH = minChatViewportLines
	}
	if m.viewport.Height != vpH {
		m.viewport.Height = vpH
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	vpW := m.chatViewportWidth(w)
	if m.viewport.Width != vpW {
		m.viewport.Width = vpW
	}
	return m
}

// measureInputBoxLines returns rendered input box height (borders + content).
func (m chatModel) measureInputBoxLines(footerW int) int {
	view := m.input.View()
	if m.useConfigInput {
		view = m.configInput.View()
	}
	box := inputBorderStyle.Width(footerW).Render(view)
	lines := strings.Split(strings.TrimRight(box, "\n"), "\n")
	if len(lines) == 0 {
		return 3
	}
	return len(lines)
}
