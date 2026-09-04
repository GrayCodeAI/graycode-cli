package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/engine"
)

func TestHandleMentions_BlocksSensitivePaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sensitive := filepath.Join(dir, ".env")
	if err := os.WriteFile(sensitive, []byte("SECRET=top-secret"), 0o600); err != nil {
		t.Fatalf("write sensitive file: %v", err)
	}

	m := &chatModel{session: &engine.Session{}}
	got := m.handleMentions(`explain @"` + sensitive + `" please`)

	if strings.Contains(got, sensitive) {
		t.Fatalf("cleaned input still contains mention: %q", got)
	}
	if len(m.messages) != 1 {
		t.Fatalf("expected 1 UI message, got %d", len(m.messages))
	}
	if m.messages[0].role != "error" {
		t.Fatalf("expected error message, got role %q", m.messages[0].role)
	}
	if !strings.Contains(m.messages[0].content, "Refused to read") {
		t.Fatalf("expected refusal message, got %q", m.messages[0].content)
	}
	if !strings.Contains(m.messages[0].content, ".env") {
		t.Fatalf("expected blocked path detail, got %q", m.messages[0].content)
	}
}
