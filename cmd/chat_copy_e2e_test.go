package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

// runCopySelectionE2EPass exercises chat + input copy/select/mouse flows in one pass.
func runCopySelectionE2EPass(t *testing.T, pass int) {
	t.Helper()

	m := newTestChatModel()
	m.input = textarea.New()
	m.viewport = viewport.New(80, 10)
	m.uiFocus = focusPrompt

	// --- Pass A: error-only turn (no assistant reply) ---
	m.messages = []displayMsg{
		{role: "user", content: "Hi"},
		{role: "system", content: "↻ retrying after reasoning-only response (attempt 2)"},
		{role: "error", content: "The model produced internal reasoning but no reply."},
	}
	m.input.SetValue("draft in prompt")

	transcript := m.copyableTranscript()
	for _, want := range []string{"You: Hi", "error: The model produced internal reasoning", "Draft: draft in prompt"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("pass %d: transcript missing %q:\n%s", pass, want, transcript)
		}
	}

	if content, label, ok := m.smartCopyContent(); !ok || label != "input" || content != "draft in prompt" {
		t.Fatalf("pass %d: smartCopy = (%q,%q,%v)", pass, content, label, ok)
	}

	result, _ := m.handleCommand("/copy input")
	cm, ok := result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy input returned %T", pass, result)
	}
	if !strings.Contains(lastSystemMessage(cm.messages), "Copied input") {
		t.Fatalf("pass %d: /copy input: %s", pass, lastSystemMessage(cm.messages))
	}
	m = cm

	result, _ = m.handleCommand("/copy all")
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy all returned %T", pass, result)
	}
	if !strings.Contains(lastSystemMessage(cm.messages), "Copied chat transcript") {
		t.Fatalf("pass %d: /copy all: %s", pass, lastSystemMessage(cm.messages))
	}
	m = cm

	result, _ = m.handleCommand("/copy")
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy returned %T", pass, result)
	}
	if last := lastSystemMessage(cm.messages); !strings.Contains(last, "Copied") {
		t.Fatalf("pass %d: /copy smart: %s", pass, last)
	}
	m = cm

	// Keyboard shortcut path
	result, _ = m.handleCopyShortcut()
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: handleCopyShortcut returned %T", pass, result)
	}
	if !strings.Contains(lastSystemMessage(cm.messages), "Copied input") {
		t.Fatalf("pass %d: Ctrl+Shift+C shortcut: %s", pass, lastSystemMessage(cm.messages))
	}
	m = cm

	if !isCopyToClipboardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true}) {
		t.Fatalf("pass %d: alt+c should be copy shortcut", pass)
	}

	// Select mode transcript (no terminal required)
	if got := m.copyableTranscript(); !strings.Contains(got, "Draft: draft in prompt") {
		t.Fatalf("pass %d: select transcript missing draft", pass)
	}

	// --- Mouse toggle (OpenCode-style) ---
	t.Setenv("HAWK_MOUSE", "")
	m.handleMouseCommand([]string{"/mouse", "off"})
	if m.mouseEnabled() {
		t.Fatalf("pass %d: expected mouse off after /mouse off", pass)
	}
	*m = m.syncViewportMouseWheel()
	if m.viewport.MouseWheelEnabled {
		t.Fatalf("pass %d: viewport auto-wheel must stay off", pass)
	}

	m.handleMouseCommand([]string{"/mouse", "on"})
	if !m.mouseEnabled() {
		t.Fatalf("pass %d: expected mouse on after /mouse on", pass)
	}

	m.handleMouseCommand([]string{"/mouse", "toggle"})
	if m.mouseEnabled() {
		t.Fatalf("pass %d: expected mouse off after toggle from on", pass)
	}

	// --- Pass B: assistant reply path ---
	m.messages = append(m.messages, displayMsg{role: "assistant", content: "Hello from hawk"})
	m.input.SetValue("")

	if content, _, ok := m.copyContent(copyModeAssistant); !ok || content != "Hello from hawk" {
		t.Fatalf("pass %d: /copy assistant content = %q ok=%v", pass, content, ok)
	}
	if line, ok := m.lastMessageContent(); !ok || !strings.Contains(line, "Hello from hawk") {
		t.Fatalf("pass %d: last message = %q ok=%v", pass, line, ok)
	}

	result, _ = m.handleCommand("/copy assistant")
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy assistant returned %T", pass, result)
	}
	if !strings.Contains(lastSystemMessage(cm.messages), "Copied assistant reply") {
		t.Fatalf("pass %d: /copy assistant: %s", pass, lastSystemMessage(cm.messages))
	}

	// Settings-backed mouse default
	disabled := false
	m2 := chatModel{settings: hawkconfig.Settings{TuiMouse: &disabled}}
	if m2.mouseEnabled() {
		t.Fatalf("pass %d: settings tui_mouse=false should disable capture", pass)
	}
}

func lastSystemMessage(msgs []displayMsg) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].role == "system" || msgs[i].role == "error" {
			return msgs[i].content
		}
	}
	return ""
}

func TestCopySelectionE2E(t *testing.T) {
	runCopySelectionE2EPass(t, 1)
	runCopySelectionE2EPass(t, 2)
}
