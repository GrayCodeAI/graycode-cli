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

func TestUpdate_BlurMsg_BlursPromptInput(t *testing.T) {
	m := chatModel{
		uiFocus:  focusPrompt,
		input:    textarea.New(),
		viewport: viewport.New(80, 10),
	}
	m.input.Focus()

	nextModel, _ := m.Update(tea.BlurMsg{})
	next := nextModel.(chatModel)
	if next.input.Focused() {
		t.Fatal("prompt input should blur on terminal blur")
	}
}

func TestUpdate_FocusMsg_RefocusesPromptInput(t *testing.T) {
	m := chatModel{
		uiFocus:  focusPrompt,
		input:    textarea.New(),
		viewport: viewport.New(80, 10),
	}
	m.input.Blur()

	nextModel, cmd := m.Update(tea.FocusMsg{})
	next := nextModel.(chatModel)
	if cmd == nil {
		t.Fatal("focus regain should schedule prompt refocus command")
	}
	if !next.input.Focused() {
		t.Fatal("prompt input should refocus on terminal focus")
	}
}

func TestUpdate_PromptKeepAlive_RefocusesBlurredPrompt(t *testing.T) {
	m := chatModel{
		uiFocus:  focusPrompt,
		input:    textarea.New(),
		viewport: viewport.New(80, 10),
	}
	m.input.Blur()

	nextModel, cmd := m.Update(promptKeepAliveMsg{})
	next := nextModel.(chatModel)
	if cmd == nil {
		t.Fatal("prompt keepalive should reschedule itself")
	}
	if !next.input.Focused() {
		t.Fatal("prompt keepalive should refocus the prompt when idle")
	}
}

func TestChatProgramOptions_IncludeFocusReporting(t *testing.T) {
	withMouse := tea.NewProgram(chatModel{}, chatProgramOptions(true)...)
	withoutMouse := tea.NewProgram(chatModel{}, chatProgramOptions(false)...)

	if withMouse == nil || withoutMouse == nil {
		t.Fatal("expected program construction to succeed")
	}
	if len(chatProgramOptions(true)) != 3 {
		t.Fatalf("mouse-enabled startup should include alt-screen, focus reporting, and mouse motion; got %d options", len(chatProgramOptions(true)))
	}
	if len(chatProgramOptions(false)) != 2 {
		t.Fatalf("mouse-disabled startup should include alt-screen and focus reporting; got %d options", len(chatProgramOptions(false)))
	}
}
