package cmd

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

func TestView_LineCountMatchesHeight(t *testing.T) {
	m := chatModel{
		height:       24,
		width:        80,
		welcomeCache: "HAWK LOGO\nv0.1.0",
		input:        textarea.New(),
		viewport:     viewport.New(viewport.WithWidth(80), viewport.WithHeight(8)),
		ghostText:    NewGhostText(),
	}
	m = m.withSyncedLayout()
	got := m.View().Content
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
	// Footer must start on the same row View() renders the top footer line.
	footerIdx := -1
	for i, line := range lines {
		if strings.Contains(line, "Docker:") {
			footerIdx = i - 1
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

func TestView_FooterVisibleWhenOutputAreaIsLargeOrMultiline(t *testing.T) {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
	vp.SetContent(strings.Repeat("Output line\n", 100))
	vp.SetYOffset(10) // Scroll down so AtTop() is false and sticky header activates

	inp := textarea.New()
	inp.SetValue("First line\nSecond line") // Multiline prompt

	m := chatModel{
		height:   24,
		width:    80,
		viewport: vp,
		input:    inp,
		messages: []displayMsg{
			{role: "user", content: "Previous user prompt that is scrolled up"},
			{role: "assistant", content: "Assistant response"},
		},
	}
	m = m.withSyncedLayout()
	got := m.View().Content
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	if len(lines) > m.height {
		t.Fatalf("total rendered view lines (%d) exceeded terminal height (%d)", len(lines), m.height)
	}

	// Verify footer is rendered within the visible terminal height
	footerFound := false
	for i, line := range lines {
		if strings.Contains(line, "Docker:") || strings.Contains(line, "tokens") || strings.Contains(line, "cost") {
			footerFound = true
			if i >= m.height {
				t.Fatalf("footer line at index %d is beyond terminal height %d", i, m.height)
			}
		}
	}
	if !footerFound {
		t.Fatal("expected footer in view output")
	}
}

func TestMouseWheelDelta_SGRUsesZeroBasedY(t *testing.T) {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(14))
	vp.SetContent(strings.Repeat("line\n", 40))
	m := chatModel{
		viewport: vp,
		input:    textarea.New(),
		height:   24,
		width:    80,
		uiFocus:  focusPrompt,
	}
	m = m.withSyncedLayout()
	before := m.viewport.YOffset()
	footerRow1Based := m.footerTopY() + 1
	chatRow1Based := m.chatPaneTopY() + 2

	leakChat := tea.KeyPressMsg{Code: '[', Text: "[<65;40;" + strconv.Itoa(chatRow1Based) + "M"}
	if handled, _ := m.tryScrollFromMouseLeak(leakChat); !handled {
		t.Fatal("expected chat wheel leak to be consumed")
	}
	if m.viewport.YOffset() == before {
		t.Fatal("SGR chat wheel should scroll viewport")
	}

	m.viewport.SetYOffset(before)
	leakInput := tea.KeyPressMsg{Code: '[', Text: "[<65;40;" + strconv.Itoa(footerRow1Based) + "M"}
	if handled, _ := m.tryScrollFromMouseLeak(leakInput); !handled {
		t.Fatal("expected footer wheel leak to be consumed")
	}
	if m.viewport.YOffset() != before {
		t.Fatal("SGR footer wheel must not scroll chat")
	}
}
