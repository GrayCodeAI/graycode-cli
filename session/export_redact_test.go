package session

import (
	"os"
	"testing"
	"time"
)

func TestRedactSecrets(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"sk-ant-api01-abc123def456",
		"normal text",
		"",
		"OPENAI_KEY=sk-test123",
	}
	for _, input := range inputs {
		_ = RedactSecrets(input) // just verify no panic
	}
}

func TestRedactSessionMessages(t *testing.T) {
	sess := &Session{
		Messages: []Message{
			{Role: "user", Content: "my key is sk-ant-api01-secret123456"},
			{Role: "assistant", Content: "I see your key"},
			{Role: "user", Content: "no secrets here"},
		},
	}
	redacted := redactSessionMessages(sess)
	if redacted == nil {
		t.Fatal("redacted should not be nil")
	}
}

func TestExport_JSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk/sessions", 0o755)

	sess := &Session{
		ID: "export-json", Model: "test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Messages: []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}},
	}
	_ = Save(sess)

	exported, err := Export(sess, "json", false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(exported) == 0 {
		t.Error("should produce output")
	}
}

func TestExport_Markdown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(dir+"/.hawk/sessions", 0o755)

	sess := &Session{
		ID: "export-md", Model: "test", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Messages: []Message{{Role: "user", Content: "explain"}, {Role: "assistant", Content: "sure"}},
	}
	_ = Save(sess)

	exported, err := Export(sess, "md", true)
	if err != nil {
		t.Fatalf("Export markdown: %v", err)
	}
	if len(exported) == 0 {
		t.Error("should produce output")
	}
}
