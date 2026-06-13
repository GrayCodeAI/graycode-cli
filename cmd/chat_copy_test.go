package cmd

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	hawkconfig "github.com/GrayCodeAI/hawk/internal/config"
)

func TestCopyableTranscript_IncludesInputDraft(t *testing.T) {
	t.Parallel()

	m := chatModel{
		input: textarea.New(),
		messages: []displayMsg{
			{role: "user", content: "Hi"},
		},
	}
	m.input.SetValue("draft prompt")

	got := m.copyableTranscript()
	if !strings.Contains(got, "You: Hi") || !strings.Contains(got, "Draft: draft prompt") {
		t.Fatalf("copyableTranscript() = %q", got)
	}
}

func TestSmartCopyContent_PrefersInputDraft(t *testing.T) {
	t.Parallel()

	m := chatModel{
		uiFocus: focusPrompt,
		input:   textarea.New(),
		messages: []displayMsg{
			{role: "assistant", content: "hello"},
		},
	}
	m.input.SetValue("typing…")

	content, label, ok := m.smartCopyContent()
	if !ok || label != "input" || content != "typing…" {
		t.Fatalf("smartCopyContent() = (%q, %q, %v)", content, label, ok)
	}
}

func TestCopyContent_Modes(t *testing.T) {
	t.Parallel()

	m := chatModel{
		input: textarea.New(),
		messages: []displayMsg{
			{role: "user", content: "Hi"},
			{role: "assistant", content: "hello"},
		},
	}
	m.input.SetValue("draft")

	if content, _, ok := m.copyContent(copyModeAssistant); !ok || content != "hello" {
		t.Fatalf("assistant mode = (%q, %v)", content, ok)
	}
	if content, _, ok := m.copyContent(copyModeInput); !ok || content != "draft" {
		t.Fatalf("input mode = (%q, %v)", content, ok)
	}
	if content, _, ok := m.copyContent(copyModeAll); !ok || !strings.Contains(content, "Draft: draft") {
		t.Fatalf("all mode = (%q, %v)", content, ok)
	}
}

func TestIsCopyToClipboardKey(t *testing.T) {
	t.Parallel()

	if !isCopyToClipboardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}, Alt: true}) {
		t.Fatal("expected alt+c")
	}
	if isCopyToClipboardKey(tea.KeyMsg{Type: tea.KeyCtrlC}) {
		t.Fatal("ctrl+c should not trigger clipboard copy")
	}
}

func TestMouseEnabled_SettingsAndEnv(t *testing.T) {
	t.Setenv("HAWK_MOUSE", "")
	disabled := false
	m := chatModel{settings: hawkconfig.Settings{TuiMouse: &disabled}}
	if m.mouseEnabled() {
		t.Fatal("expected settings tui_mouse=false to disable capture")
	}

	t.Setenv("HAWK_MOUSE", "0")
	if m.mouseEnabled() {
		t.Fatal("expected HAWK_MOUSE=0 to disable capture")
	}

	t.Setenv("HAWK_MOUSE", "1")
	if !m.mouseEnabled() {
		t.Fatal("expected HAWK_MOUSE=1 to enable capture")
	}
}
