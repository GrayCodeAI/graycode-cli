package cmd

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	graycodeconfig "github.com/GrayCodeAI/graycode-cli/internal/config"
)

// runCopySelectionE2EPass exercises chat + input copy/select/mouse flows in one pass.
func runCopySelectionE2EPass(t *testing.T, pass int) {
	t.Helper()

	m := newTestChatModel()
	m.input = textarea.New()
	m.viewport = viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
	m.uiFocus = focusPrompt

	// --- Pass A: error-only turn (no assistant reply) ---
	m.messages = []displayMsg{
		{role: "user", content: "Hi"},
		{role: "system", content: "↻ retrying after reasoning-only response (attempt 2)"},
		{role: "error", content: "The model produced internal reasoning but no reply."},
	}
	m.input.SetValue("draft in prompt")

	transcript := m.copyableTranscript()
	for _, want := range []string{"You: Hi", "Error: The model produced internal reasoning", "Draft: draft in prompt"} {
		if !strings.Contains(transcript, want) {
			t.Fatalf("pass %d: transcript missing %q:\n%s", pass, want, transcript)
		}
	}

	if content, label, got := m.smartCopyContent(); !got || label != "input" || content != "draft in prompt" {
		t.Fatalf("pass %d: smartCopy = (%q,%q,%v)", pass, content, label, got)
	}

	result, _ := m.handleCommand("/copy input")
	cm, ok := result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy input returned %T", pass, result)
	}
	copyInputMsg := lastSystemMessage(cm.messages)
	if strings.Contains(copyInputMsg, "Failed to copy") {
		if result := copyToClipboard("probe"); result.FallbackPath == "" {
			t.Skipf("pass %d: clipboard not available on runner (no fallback path either)", pass)
		}
		t.Fatalf("pass %d: /copy input: %s", pass, copyInputMsg)
	}
	m = cm

	result, _ = m.handleCommand("/copy all")
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy all returned %T", pass, result)
	}
	assertCopySucceeded(t, pass, "/copy all", lastSystemMessage(cm.messages), "Copied chat transcript")
	m = cm

	result, _ = m.handleCommand("/copy")
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy returned %T", pass, result)
	}
	assertCopySucceeded(t, pass, "/copy smart", lastSystemMessage(cm.messages), "Copied")
	m = cm

	// Keyboard shortcut path
	result, _ = m.handleCopyShortcut()
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: handleCopyShortcut returned %T", pass, result)
	}
	assertCopySucceeded(t, pass, "Ctrl+Shift+C shortcut", lastSystemMessage(cm.messages), "Copied input")
	m = cm

	if !isCopyToClipboardKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt}) {
		t.Fatalf("pass %d: alt+c should be copy shortcut", pass)
	}

	// Select mode transcript (no terminal required)
	if got := m.copyableTranscript(); !strings.Contains(got, "Draft: draft in prompt") {
		t.Fatalf("pass %d: select transcript missing draft", pass)
	}

	// --- Mouse toggle (OpenCode-style) ---
	t.Setenv("GRAYCODE_MOUSE", "")
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
	m.messages = append(m.messages, displayMsg{role: "assistant", content: "Hello from graycode"})
	m.input.SetValue("")

	if content, _, got := m.copyContent(copyModeAssistant); !got || content != "Hello from graycode" {
		t.Fatalf("pass %d: /copy assistant content = %q got=%v", pass, content, got)
	}
	if line, got := m.lastMessageContent(); !got || !strings.Contains(line, "Hello from graycode") {
		t.Fatalf("pass %d: last message = %q got=%v", pass, line, got)
	}

	result, _ = m.handleCommand("/copy assistant")
	cm, ok = result.(*chatModel)
	if !ok {
		t.Fatalf("pass %d: /copy assistant returned %T", pass, result)
	}
	assertCopySucceeded(t, pass, "/copy assistant", lastSystemMessage(cm.messages), "Copied assistant reply")

	// Settings-backed mouse default
	disabled := false
	m2 := chatModel{settings: graycodeconfig.Settings{TuiMouse: &disabled}}
	if m2.mouseEnabled() {
		t.Fatalf("pass %d: settings tui_mouse=false should disable capture", pass)
	}
}

func assertCopySucceeded(t *testing.T, pass int, operation, message, copiedText string) {
	t.Helper()
	if strings.Contains(message, copiedText) || strings.Contains(message, "Clipboard unavailable — saved") {
		return
	}
	t.Fatalf("pass %d: %s: %s", pass, operation, message)
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
