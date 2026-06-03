package cmd

// chatScrollbarVisible reports when chat content exceeds the viewport (scrollback available).
// Used for footer scroll position only — no separate slider column in the UI.
func (m chatModel) chatScrollbarVisible() bool {
	h := m.viewport.Height
	if h <= 0 {
		return false
	}
	return m.contentLines > h
}

func (m chatModel) chatViewportWidth(totalWidth int) int {
	if totalWidth < 20 {
		return 80
	}
	return totalWidth
}

func (m chatModel) renderChatPane() string {
	return m.viewport.View()
}