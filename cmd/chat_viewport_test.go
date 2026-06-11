package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestRouteKeyToViewport_ArrowsInPromptFocus(t *testing.T) {
	vp := viewport.New(80, 10)
	vp.SetContent(strings.Repeat("line\n", 40))
	vp.SetYOffset(5)

	ta := textarea.New()
	m := chatModel{viewport: vp, input: ta, uiFocus: focusPrompt}
	up := tea.KeyMsg{Type: tea.KeyUp}
	if m.routeKeyToViewport(up) {
		t.Fatal("up in prompt focus should use input history, not scroll chat")
	}
	m.uiFocus = focusScrollback
	if !m.routeKeyToViewport(up) {
		t.Fatal("up in scrollback focus should scroll when not at top")
	}
}

func TestMouseInChatPane_Zones(t *testing.T) {
	vp := viewport.New(80, 14)
	m := chatModel{
		viewport: vp,
		input:    textarea.New(),
		height:   24,
		width:    80,
		uiFocus:  focusPrompt,
	}
	m = m.withSyncedLayout()

	top := m.chatPaneTopY()
	bottom := m.bottomBarTopY()
	if bottom <= top {
		t.Fatalf("invalid zones top=%d bottom=%d", top, bottom)
	}

	overChat := tea.MouseMsg{Y: top, Button: tea.MouseButtonWheelDown}
	if !m.mouseInChatPane(overChat) {
		t.Fatal("expected wheel row on chat pane")
	}
	overInput := tea.MouseMsg{Y: bottom, Button: tea.MouseButtonWheelDown}
	if m.mouseInChatPane(overInput) {
		t.Fatal("expected wheel row on input footer to be outside chat pane")
	}
}

func TestShouldRouteMouseToViewport_SplitPaneUX(t *testing.T) {
	vp := viewport.New(80, 14)
	vp.SetContent(strings.Repeat("line\n", 40))
	m := chatModel{
		viewport: vp,
		input:    textarea.New(),
		height:   24,
		width:    80,
		uiFocus:  focusPrompt,
	}
	m = m.withSyncedLayout()

	wheelChat := tea.MouseMsg{Y: m.chatPaneTopY(), Button: tea.MouseButtonWheelDown}
	wheelInput := tea.MouseMsg{Y: m.bottomBarTopY(), Button: tea.MouseButtonWheelDown}

	if !m.shouldRouteMouseToViewport(wheelChat) {
		t.Fatal("wheel over chat should scroll history in prompt focus")
	}
	if m.shouldRouteMouseToViewport(wheelInput) {
		t.Fatal("wheel over input should not scroll chat in prompt focus")
	}

	m.uiFocus = focusScrollback
	if !m.shouldRouteMouseToViewport(wheelInput) {
		t.Fatal("wheel should scroll in scrollback focus anywhere")
	}
}

func TestSyncViewportMouseWheel_EnabledInWorkMode(t *testing.T) {
	vp := viewport.New(80, 10)
	m := chatModel{viewport: vp, uiFocus: focusPrompt, phase: phaseWork}
	m = m.syncViewportMouseWheel()
	if !m.viewport.MouseWheelEnabled {
		t.Fatal("wheel should be enabled in work mode (zone routing applies)")
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
