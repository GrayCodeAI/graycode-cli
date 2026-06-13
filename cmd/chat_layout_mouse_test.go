package cmd

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestView_LineCountMatchesHeight(t *testing.T) {
	m := chatModel{
		height:       24,
		width:        80,
		welcomeCache: "HAWK LOGO\nv0.1.0",
		input:        textarea.New(),
		viewport:     viewport.New(80, 8),
		ghostText:    NewGhostText(),
		phase:        phaseWork,
	}
	m = m.withSyncedLayout()
	got := m.View()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) > m.height {
		t.Fatalf("view lines = %d, must not exceed height %d", len(lines), m.height)
	}
	if m.footerTopY() >= m.height {
		t.Fatalf("footerTopY %d must be within height %d", m.footerTopY(), m.height)
	}
	if m.footerTopY() <= m.chatPaneTopY() {
		t.Fatalf("footerTopY %d must be below chat top %d", m.footerTopY(), m.chatPaneTopY())
	}
	// Footer must start on the same row View() renders the container/model line.
	footerIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Default") || strings.Contains(line, "Container:") {
			footerIdx = i
			break
		}
	}
	if footerIdx < 0 {
		t.Fatal("expected footer row in view")
	}
	if footerIdx != m.footerTopY() {
		t.Fatalf("view footer row %d != footerTopY %d", footerIdx, m.footerTopY())
	}
}

func TestMouseWheelDelta_SGRUsesZeroBasedY(t *testing.T) {
	vp := viewport.New(80, 14)
	vp.SetContent(strings.Repeat("line\n", 40))
	m := chatModel{
		viewport: vp,
		input:    textarea.New(),
		height:   24,
		width:    80,
		uiFocus:  focusPrompt,
		phase:    phaseWork,
	}
	m = m.withSyncedLayout()
	before := m.viewport.YOffset
	footerRow1Based := m.footerTopY() + 1
	chatRow1Based := m.chatPaneTopY() + 2

	leakChat := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<65;40;" + strconv.Itoa(chatRow1Based) + "M")}
	if handled, _ := m.tryScrollFromMouseLeak(leakChat); !handled {
		t.Fatal("expected chat wheel leak to be consumed")
	}
	if m.viewport.YOffset == before {
		t.Fatal("SGR chat wheel should scroll viewport")
	}

	m.viewport.SetYOffset(before)
	leakInput := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<65;40;" + strconv.Itoa(footerRow1Based) + "M")}
	if handled, _ := m.tryScrollFromMouseLeak(leakInput); !handled {
		t.Fatal("expected footer wheel leak to be consumed")
	}
	if m.viewport.YOffset != before {
		t.Fatal("SGR footer wheel must not scroll chat")
	}
}
