package cmd

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestArrowBurstDoesNotPermanentlyFreezeInput reproduces a reported bug: after
// a rapid burst of Up/Down history navigation, the input box would stop
// accepting any further keystrokes (including Escape) forever. Root cause was
// arrowBurstActive getting set true on the burst's last keypress with no
// trailing tick ever scheduled to clear it, combined with
// applyPromptArrowKey swallowing every key type (not just arrows) while the
// flag was set.
func TestArrowBurstDoesNotPermanentlyFreezeInput(t *testing.T) {
	m := newTestChatModel()
	m.uiFocus = focusPrompt
	m.input.Focus()

	// First Up: treated as an isolated press (dt since zero-value lastArrowTime
	// is huge), so it arms a pendingArrow + tick and returns immediately.
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	cm := requireChatModel(t, next)
	if cmd == nil {
		t.Fatal("expected a scheduled tick command after the first arrow press")
	}

	// Second Up arriving <30ms later: this is the burst path. It flushes the
	// pending arrow and, critically, must arm its own trailing tick so the
	// flag can be cleared once the burst goes quiet.
	cm.lastArrowTime = time.Now().Add(-1 * time.Millisecond)
	next, cmd = cm.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	cm = requireChatModel(t, next)
	if !cm.arrowBurstActive {
		t.Fatal("expected arrowBurstActive to be true immediately after a burst keypress")
	}
	if cmd == nil {
		t.Fatal("expected the burst keypress to arm a trailing cleanup tick")
	}

	// Simulate that trailing tick firing with no further arrow keys in between.
	seq := cm.arrowSeq
	next, _ = cm.Update(processArrowTickMsg{seq: seq})
	cm = requireChatModel(t, next)
	if cm.arrowBurstActive {
		t.Fatal("arrowBurstActive should be cleared once the burst's trailing tick fires")
	}

	// Regression check: a normal character keystroke must reach the input,
	// not be silently swallowed.
	next, _ = cm.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	cm = requireChatModel(t, next)
	if cm.input.Value() != "x" {
		t.Fatalf("expected keystroke to reach the input after burst settled, got %q", cm.input.Value())
	}
}

// TestArrowBurstActiveOnlySwallowsArrowKeys guards the narrower fix directly:
// even while arrowBurstActive is true, only Up/Down should be treated as
// already-consumed — every other key must still be forwarded to the input.
func TestArrowBurstActiveOnlySwallowsArrowKeys(t *testing.T) {
	m := newTestChatModel()
	m.uiFocus = focusPrompt
	m.arrowBurstActive = true

	if !m.applyPromptArrowKey(tea.KeyPressMsg{Code: tea.KeyUp}) {
		t.Fatal("expected Up to be consumed while a burst is active")
	}
	if m.applyPromptArrowKey(tea.KeyPressMsg{Code: tea.KeyEsc}) {
		t.Fatal("Escape must not be swallowed just because an arrow burst is active")
	}
	if m.applyPromptArrowKey(tea.KeyPressMsg{Code: 'a', Text: "a"}) {
		t.Fatal("typed characters must not be swallowed just because an arrow burst is active")
	}
}
