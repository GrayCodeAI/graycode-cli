package cmd

import (
	"strings"
)

const minChatViewportLines = 4

// renderWelcomeScreen returns the branded welcome pane to be prepended to the chat viewport.
func (m chatModel) renderWelcomeScreen(width int) string {
	if strings.TrimSpace(m.welcomeCache) == "" {
		return ""
	}
	return strings.TrimRight(m.welcomeCache, "\n")
}

// withSyncedLayout returns m with viewport size reserved for welcome + bottom chrome.
func (m chatModel) withSyncedLayout() chatModel {
	if m.height <= 0 {
		return m
	}
	bottomH := m.chatBottomBarLines()
	// Viewport takes all available space above the bottom bar.
	vpH := m.height - bottomH
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
	box = clipRenderedBlock(box, footerW)
	lines := strings.Split(strings.TrimRight(box, "\n"), "\n")
	if len(lines) == 0 {
		return 3
	}
	return len(lines)
}
