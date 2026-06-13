package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func runMouseScrollSplitPanePass(t *testing.T, pass int) {
	t.Helper()

	vp := viewport.New(80, 14)
	vp.SetContent(strings.Repeat("line\n", 40))
	vp.SetYOffset(5)

	ta := textarea.New()
	ta.SetHeight(1)
	m := chatModel{
		viewport: vp,
		input:    ta,
		height:   24,
		width:    80,
		uiFocus:  focusPrompt,
		phase:    phaseWork,
	}
	m = m.syncViewportMouseWheel().withSyncedLayout()
	before := m.viewport.YOffset

	wheelChat := tea.MouseMsg{
		X:      40,
		Y:      m.chatPaneTopY(),
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	}
	next, _ := m.Update(wheelChat)
	m = next.(chatModel)
	if m.viewport.YOffset <= before {
		t.Fatalf("pass %d: wheel over chat should scroll viewport (before=%d after=%d)", pass, before, m.viewport.YOffset)
	}

	m.viewport.SetYOffset(before)
	wheelInput := tea.MouseMsg{
		X:      40,
		Y:      m.bottomBarTopY(),
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	}
	next, _ = m.Update(wheelInput)
	m = next.(chatModel)
	if m.viewport.YOffset != before {
		t.Fatalf("pass %d: wheel over input must not scroll chat (before=%d after=%d)", pass, before, m.viewport.YOffset)
	}
	if !m.input.Focused() {
		t.Fatalf("pass %d: input must stay focused after mouse wheel so typing still works", pass)
	}

	up := tea.KeyMsg{Type: tea.KeyUp}
	m.history = []string{"first", "second"}
	m.historyIdx = len(m.history)
	m.input.SetValue("")
	if m.routeKeyToViewport(up) {
		t.Fatalf("pass %d: up in prompt focus should not route to viewport", pass)
	}
	next, _ = m.Update(up)
	m = next.(chatModel)
	if m.input.Value() != "second" {
		t.Fatalf("pass %d: up should navigate input history, got %q", pass, m.input.Value())
	}
	if m.viewport.YOffset != before {
		t.Fatalf("pass %d: up in prompt focus must not scroll chat", pass)
	}
}

func TestUpdate_MouseWheelSplitPane(t *testing.T) {
	runMouseScrollSplitPanePass(t, 1)
	runMouseScrollSplitPanePass(t, 2)
}

func TestUpdate_InputHistoryWhileWaiting(t *testing.T) {
	m := chatModel{
		viewport: viewport.New(80, 14),
		input:    textarea.New(),
		height:   24,
		width:    80,
		uiFocus:  focusPrompt,
		phase:    phaseWork,
		waiting:  true,
		history:  []string{"first", "second"},
	}
	m.historyIdx = len(m.history)
	m = m.withSyncedLayout()

	up := tea.KeyMsg{Type: tea.KeyUp}
	next, _ := m.Update(up)
	m = next.(chatModel)
	if m.input.Value() != "second" {
		t.Fatalf("up while waiting should navigate history, got %q", m.input.Value())
	}

	down := tea.KeyMsg{Type: tea.KeyDown}
	next, _ = m.Update(down)
	m = next.(chatModel)
	if m.input.Value() != "" {
		t.Fatalf("down while waiting should restore empty draft, got %q", m.input.Value())
	}
}
