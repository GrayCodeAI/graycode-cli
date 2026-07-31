package cmd

import (
	"strings"
	"testing"
)

func TestPlainTranscript(t *testing.T) {
	t.Parallel()

	messages := []displayMsg{
		{role: "welcome", content: "ignored"},
		{role: "user", content: "Hi"},
		{role: "system", content: "↻ retrying"},
		{role: "error", content: "model produced reasoning but no answer"},
	}
	got := plainTranscript(messages, "")
	want := strings.Join([]string{
		"You: Hi",
		"↻ retrying",
		"Error: model produced reasoning but no answer",
	}, "\n\n")
	if got != want {
		t.Fatalf("plainTranscript() = %q, want %q", got, want)
	}
}

func TestLastCopyableContent_PrefersAssistant(t *testing.T) {
	t.Parallel()

	m := chatModel{
		messages: []displayMsg{
			{role: "user", content: "Hi"},
			{role: "error", content: "boom"},
			{role: "assistant", content: "hello"},
		},
	}
	got, ok := m.lastCopyableContent()
	if !ok || got != "hello" {
		t.Fatalf("lastCopyableContent() = (%q, %v), want (hello, true)", got, ok)
	}
}

func TestLastCopyableContent_FallsBackToTranscript(t *testing.T) {
	t.Parallel()

	m := chatModel{
		messages: []displayMsg{
			{role: "user", content: "Hi"},
			{role: "error", content: "boom"},
		},
	}
	got, ok := m.lastCopyableContent()
	if !ok {
		t.Fatal("expected copyable content")
	}
	if !strings.Contains(got, "You: Hi") || !strings.Contains(got, "Error: boom") {
		t.Fatalf("lastCopyableContent() = %q, want transcript fallback", got)
	}
}
