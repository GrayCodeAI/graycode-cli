package engine

import (
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/securitylog"
	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestSessionRecordsSecurityDenial(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", dir)

	s := &Session{life: NewLifecycleService(nil)}
	s.recordSecurityDenial(types.ToolCall{Name: "Bash", ID: "tc1"}, "permission", "denied by policy ceiling")

	// DefaultDir honors GRAYCODE_STATE_DIR set in this test.
	logDir := securitylog.DefaultDir()
	events, err := securitylog.Entries(logDir)
	if err != nil {
		t.Fatalf("reading events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 security event, got %d", len(events))
	}
	if events[0].Type != "denied" {
		t.Fatalf("expected event type %q, got %q", "denied", events[0].Type)
	}
	if events[0].Tool != "Bash" {
		t.Fatalf("expected tool Bash, got %q", events[0].Tool)
	}
	// Chain must verify.
	if count, err := securitylog.Verify(logDir); err != nil || count != 1 {
		t.Fatalf("chain verification: %d events, err=%v", count, err)
	}
}

func TestSessionSecurityDenialIsNilSafe(t *testing.T) {
	var s *Session
	s.recordSecurityDenial(types.ToolCall{Name: "Bash"}, "permission", "")
}

func TestDefaultDirHonorsStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", dir)
	expected := filepath.Join(dir, "securitylog")
	if got := securitylog.DefaultDir(); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}
