package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func TestFormatToolResultDisplay_TruncatesLarge(t *testing.T) {
	long := strings.Repeat("x", toolDisplayMaxChars+5000)
	out := formatToolResultDisplay(long)
	if len(out) >= len(long) {
		t.Fatal("expected truncated tool display")
	}
	if !strings.Contains(out, "omitted") && !strings.Contains(out, "more chars") {
		t.Fatalf("expected omission hint, got len=%d", len(out))
	}
}

func TestScrollPositionLabel(t *testing.T) {
	vp := viewport.New(80, 10)
	vp.SetContent(strings.Repeat("line\n", 50))
	vp.SetYOffset(10)
	m := chatModel{viewport: vp, contentLines: 51}
	got := m.scrollPositionLabel()
	if !strings.Contains(got, "11-") || !strings.Contains(got, "/51") {
		t.Fatalf("unexpected position label: %q", got)
	}
}

func TestRouteKeyToViewport_ScrollbackFocus(t *testing.T) {
	vp := viewport.New(80, 10)
	vp.SetContent(strings.Repeat("line\n", 30))
	m := chatModel{viewport: vp, uiFocus: focusScrollback}
	if m.routeKeyToViewport(tea.KeyMsg{Type: tea.KeyUp}) {
		t.Fatal("up should NOT scroll in scrollback focus")
	}
}

func TestCloseConfigPanel_ReturnsPromptFocus(t *testing.T) {
	m := chatModel{configOpen: true, uiFocus: focusScrollback, input: textarea.New()}
	next := m.closeConfigPanel()
	if next.uiFocus != focusPrompt {
		t.Fatalf("uiFocus = %v, want prompt", next.uiFocus)
	}
}

func TestUpdate_TypingInScrollbackReturnsToPrompt(t *testing.T) {
	m := chatModel{
		uiFocus:  focusScrollback,
		input:    textarea.New(),
		viewport: viewport.New(80, 10),
	}
	nextModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	next := nextModel.(chatModel)
	if next.uiFocus != focusPrompt {
		t.Fatalf("uiFocus = %v, want prompt", next.uiFocus)
	}
	if got := next.input.Value(); got != "a" {
		t.Fatalf("input = %q, want %q", got, "a")
	}
}
