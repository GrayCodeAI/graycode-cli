package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
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

func TestSyncViewportMouseWheel_ManualRouting(t *testing.T) {
	t.Setenv("HAWK_MOUSE", "")
	vp := viewport.New(80, 10)
	m := chatModel{viewport: vp, uiFocus: focusPrompt, phase: phaseWork}
	m = m.syncViewportMouseWheel()
	if m.viewport.MouseWheelEnabled {
		t.Fatal("viewport auto-wheel must stay off; hawk routes wheel by pane")
	}
}

func TestSyncViewportMouseWheel_DisabledWithOptOut(t *testing.T) {
	t.Setenv("HAWK_MOUSE", "0")
	vp := viewport.New(80, 10)
	disabled := false
	m := chatModel{viewport: vp, uiFocus: focusPrompt, phase: phaseWork, settings: hawkconfig.Settings{TuiMouse: &disabled}}
	m = m.syncViewportMouseWheel()
	if m.viewport.MouseWheelEnabled {
		t.Fatal("wheel should be disabled when mouse capture is off")
	}
}

func TestTryScrollFromMouseLeak_SplitPaneByY(t *testing.T) {
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
	before := m.viewport.YOffset

	chatLeak := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<65;99;6M")} // SGR Y is 1-based → row 5
	handled, _ := m.tryScrollFromMouseLeak(chatLeak)
	if !handled {
		t.Fatal("expected chat leak to be consumed")
	}
	if m.viewport.YOffset == before {
		t.Fatal("wheel leak over chat should scroll viewport")
	}

	m.viewport.SetYOffset(before)
	inputLeak := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<65;99;23M")} // 1-based footer row
	handled, _ = m.tryScrollFromMouseLeak(inputLeak)
	if !handled {
		t.Fatal("expected input leak to be consumed")
	}
	if m.viewport.YOffset != before {
		t.Fatal("wheel leak over input should not scroll viewport")
	}
}

func TestLetterMNotTreatedAsMouseLeak(t *testing.T) {
	for _, s := range []string{"m", "M", "hello", "lam", "vim", "make"} {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		if isMouseSequenceLeak(msg) {
			t.Fatalf("%q must not be filtered as mouse leak", s)
		}
		if shouldForwardToInput(msg) != true {
			t.Fatalf("%q must forward to input", s)
		}
	}
	if got := stripMouseLeaks("make vim lam"); got != "make vim lam" {
		t.Fatalf("stripMouseLeaks removed letters from words: %q", got)
	}
}

func TestMouseSequenceLeak_Filtered(t *testing.T) {
	leak := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[<65;49;18M")}
	if !isMouseSequenceLeak(leak) {
		t.Fatal("expected SGR mouse leak detection")
	}
	if shouldForwardToInput(leak) {
		t.Fatal("leak must not forward to input")
	}
	got := stripMouseLeaks("hi[<64;86;20M[<65;49;18Mthere")
	if got != "hithere" {
		t.Fatalf("stripMouseLeaks = %q, want hithere", got)
	}
}

func TestMouseSequenceLeak_PartialFragments(t *testing.T) {
	partials := []string{"[", "[<", "[<65", "65;99;16M"}
	for _, s := range partials {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		if !isMouseSequenceLeak(msg) {
			t.Fatalf("expected partial leak %q to be filtered", s)
		}
		if shouldForwardToInput(msg) {
			t.Fatalf("partial leak %q must not forward to input", s)
		}
	}
	if stripMouseLeaks("still[<65;99;16M") != "still" {
		t.Fatal("stripMouseLeaks should remove trailing leak")
	}
}

func TestMouseSequenceLeak_CursorConcatenated(t *testing.T) {
	// Cursor integrated terminal often drops "[" on repeated wheel events.
	leak := "[<65;84;24M[<64;84;24M<64;84;24M<64;84;24M<65;84;24M"
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(leak)}
	if !isMouseSequenceLeak(msg) {
		t.Fatal("expected concatenated Cursor leak detection")
	}
	if shouldForwardToInput(msg) {
		t.Fatal("concatenated leak must not forward to input")
	}
	if got := stripMouseLeaks(leak); got != "" {
		t.Fatalf("stripMouseLeaks = %q, want empty", got)
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
