package cmd

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
)

func newVoiceTestModel() chatModel {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(14))
	ta := textarea.New()
	ta.SetHeight(1)
	return chatModel{
		viewport: vp,
		input:    ta,
		height:   24,
		width:    80,
		uiFocus:  focusPrompt,
	}
}

// TestVoiceResultMsg verifies the /voice background result is applied to the
// model only via the update loop (the fix for the prior data race, where a raw
// goroutine mutated m.messages and m.input directly).
func TestVoiceResultMsg(t *testing.T) {
	t.Run("transcript injects input and announces", func(t *testing.T) {
		m := newVoiceTestModel()
		next, _ := m.Update(voiceResultMsg{transcript: "hello world"})
		m = next.(chatModel)
		if m.input.Value() != "hello world" {
			t.Errorf("input = %q, want %q", m.input.Value(), "hello world")
		}
		if len(m.messages) != 1 || m.messages[0].role != "system" || !strings.Contains(m.messages[0].content, "hello world") {
			t.Fatalf("expected a system voice-input message, got %+v", m.messages)
		}
	})

	t.Run("error shows error and leaves input untouched", func(t *testing.T) {
		m := newVoiceTestModel()
		next, _ := m.Update(voiceResultMsg{err: "Recording failed: boom"})
		m = next.(chatModel)
		if len(m.messages) != 1 || m.messages[0].role != "error" || !strings.Contains(m.messages[0].content, "boom") {
			t.Fatalf("expected an error message, got %+v", m.messages)
		}
		if m.input.Value() != "" {
			t.Errorf("input should be unchanged on error, got %q", m.input.Value())
		}
	})

	t.Run("info shows system message", func(t *testing.T) {
		m := newVoiceTestModel()
		next, _ := m.Update(voiceResultMsg{info: "No audio recorder found."})
		m = next.(chatModel)
		if len(m.messages) != 1 || m.messages[0].role != "system" || !strings.Contains(m.messages[0].content, "No audio recorder") {
			t.Fatalf("expected a system info message, got %+v", m.messages)
		}
	})

	t.Run("empty result is a no-op", func(t *testing.T) {
		m := newVoiceTestModel()
		next, _ := m.Update(voiceResultMsg{})
		m = next.(chatModel)
		if len(m.messages) != 0 {
			t.Fatalf("expected no messages for an empty result, got %+v", m.messages)
		}
		if m.input.Value() != "" {
			t.Errorf("input should be unchanged for an empty result, got %q", m.input.Value())
		}
	})
}
