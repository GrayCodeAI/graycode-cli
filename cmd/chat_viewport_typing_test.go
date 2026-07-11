package cmd

import (
	"strings"
	"testing"

			tea "charm.land/bubbletea/v2"
)

// TestScrollableLettersReachPromptWhenFocused reproduces a reported bug:
// once the chat transcript overflows the viewport (routeKeyToViewport's
// viewportScrollable() becomes true), routeKeyToViewport treated plain "u",
// "d", "f", "b" keystrokes as vim-style pager scroll commands unconditionally
// — even while the user was typing normally in the prompt — so those letters
// never reached the input box.
func TestScrollableLettersReachPromptWhenFocused(t *testing.T) {
	m := newTestChatModel()
	m.uiFocus = focusPrompt
	m.input.Focus()

	// Make the viewport genuinely scrollable: more content lines than height.
	m.viewport.Height = 3
	m.viewport.SetContent(strings.Repeat("line\n", 20))
	if !m.viewportScrollable() {
		t.Fatal("test setup failed: viewport should be scrollable")
	}

	for _, ch := range []string{"d", "b", "f", "u"} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(ch)})
		*m = next.(chatModel)
	}

	if got := m.input.Value(); got != "dbfu" {
		t.Fatalf("expected typed letters to reach the input, got %q", got)
	}
}
