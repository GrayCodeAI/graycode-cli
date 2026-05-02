package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditEntry_Record(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "audit.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	al := &AuditLog{f: f, path: path}
	al.Record(AuditEntry{Tool: "Write", Action: "write", Path: "/tmp/test.txt", BytesWritten: 42})
	al.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected audit entry to be written")
	}
}

func TestFormatAuditSummary_Empty(t *testing.T) {
	// FormatAuditSummary reads from ~/.hawk/audit/<today>.jsonl.
	// If no entries exist, it returns the "No file modifications" message.
	summary := FormatAuditSummary()
	// Either "No file modifications recorded today." or a real summary — both are valid.
	// We just verify it doesn't panic and returns non-empty.
	if summary == "" {
		t.Fatal("expected non-empty summary string")
	}
}
