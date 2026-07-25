package cmd

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

// stickyHeaderHeight is the maximum number of lines the sticky header occupies.
const stickyHeaderHeight = 2

// lastUserPromptBeforeScroll finds the content of the most recent user message
// that has scrolled above the visible viewport area. Returns empty if the
// viewport is at the top or no user message is found.
func (m chatModel) lastUserPromptBeforeScroll() string {
	if m.viewport.AtTop() {
		return ""
	}
	// Walk messages in reverse to find the last user role message.
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].role == "user" {
			content := strings.TrimSpace(m.messages[i].content)
			if content != "" {
				return content
			}
		}
	}
	return ""
}

// renderStickyHeader renders the sticky user-prompt header that appears at the
// top of the chat pane when the user has scrolled up past a user message.
// Returns an empty string when the header should not be shown.
func (m chatModel) renderStickyHeader(viewWidth int) string {
	prompt := m.lastUserPromptBeforeScroll()
	if prompt == "" {
		return ""
	}

	// Truncate and format the prompt for single-line display.
	// Use rune-based slicing to avoid splitting multi-byte UTF-8 characters.
	display := strings.ReplaceAll(prompt, "\n", " ")
	runes := []rune(display)
	if viewWidth < 13 {
		// Narrow terminal: just trim to viewWidth, no room for padding/ellipsis.
		if len(runes) > viewWidth {
			display = string(runes[:viewWidth])
		}
	} else if len(runes) > viewWidth-10 {
		display = string(runes[:viewWidth-13]) + "..."
	}

	// Build a compact header line with a subtle visual separator.
	// Colors are derived from the active theme's global vars so the header
	// matches the current palette.
	var b strings.Builder
	headerText := "You: " + display
	headerStyle := lipgloss.NewStyle().
		Foreground(textMuted).
		Background(bgCode).
		Width(viewWidth).
		PaddingLeft(1)
	b.WriteString(headerStyle.Render(headerText))
	b.WriteString("\n")
	// Dim separator line to visually anchor the header.
	sepStyle := lipgloss.NewStyle().Foreground(borderDim)
	b.WriteString(sepStyle.Render(strings.Repeat("─", viewWidth)))
	return b.String()
}
