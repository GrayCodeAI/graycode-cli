package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSessionMigrate(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", state)
	id := "migrate-cmd-test"
	sessDir := filepath.Join(state, "sessions")
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"id":"` + id + `","messages":[{"role":"user","content":"hi"}]}`
	if err := os.WriteFile(filepath.Join(sessDir, id+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	var sb strings.Builder
	c := sessionMigrateCmd
	c.SetOut(&sb)
	c.SetErr(&sb)
	if err := runSessionMigrate(c, []string{id}); err != nil {
		t.Fatalf("runSessionMigrate: %v", err)
	}
	if !strings.Contains(sb.String(), "Migrated session "+id) {
		t.Errorf("output = %q, want migration message", sb.String())
	}
	if _, err := os.Stat(filepath.Join(sessDir, id+".json")); !os.IsNotExist(err) {
		t.Errorf("legacy .json not removed")
	}
	if _, err := os.Stat(filepath.Join(sessDir, id+".jsonl")); err != nil {
		t.Errorf("migrated .jsonl missing: %v", err)
	}
}

func TestRunSessionMigrateJSON(t *testing.T) {
	state := t.TempDir()
	t.Setenv("GRAYCODE_STATE_DIR", state)
	id := "migrate-cmd-json"
	sessDir := filepath.Join(state, "sessions")
	_ = os.MkdirAll(sessDir, 0o700)
	body := `{"id":"` + id + `","messages":[]}`
	_ = os.WriteFile(filepath.Join(sessDir, id+".json"), []byte(body), 0o600)

	old := sessionMigrateJSON
	sessionMigrateJSON = true
	defer func() { sessionMigrateJSON = old }()

	var sb strings.Builder
	c := sessionMigrateCmd
	c.SetOut(&sb)
	c.SetErr(&sb)
	if err := runSessionMigrate(c, []string{id}); err != nil {
		t.Fatalf("runSessionMigrate: %v", err)
	}
	if !strings.Contains(sb.String(), `"from_version": 0`) {
		t.Errorf("json output missing from_version:\n%s", sb.String())
	}
}

func TestRunSessionMigrateRequiresID(t *testing.T) {
	var sb strings.Builder
	c := sessionMigrateCmd
	c.SetOut(&sb)
	c.SetErr(&sb)
	if err := runSessionMigrate(c, nil); err == nil {
		t.Fatal("expected error when no id supplied")
	}
}
