package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRouteKeyToViewport_ArrowsWhenScrollable(t *testing.T) {
	vp := viewport.New(80, 10)
	vp.SetContent(strings.Repeat("line\n", 40))
	vp.SetYOffset(5)

	ta := textarea.New()
	m := chatModel{viewport: vp, input: ta}
	up := tea.KeyMsg{Type: tea.KeyUp}
	if !m.routeKeyToViewport(up) {
		t.Fatal("expected up to scroll when not at top")
	}
	vp.GotoTop()
	m.viewport = vp
	if m.routeKeyToViewport(up) {
		t.Fatal("expected up at top to use input history, not scroll")
	}
}

func TestWelcomeHeader_AlwaysFull(t *testing.T) {
	m := chatModel{
		welcomeCache: "HAWK LOGO",
		messages: []displayMsg{
			{role: "user", content: "hi"},
			{role: "assistant", content: "hello"},
		},
	}
	got := m.renderFixedWelcomePane(80)
	if !strings.Contains(got, "HAWK LOGO") {
		t.Fatalf("welcome should remain in fixed pane: %q", got)
	}
}
