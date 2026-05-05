package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExecPrompt_Arg(t *testing.T) {
	p, err := resolveExecPrompt([]string{"hello world"})
	if err != nil {
		t.Fatal(err)
	}
	if p != "hello world" {
		t.Errorf("expected 'hello world', got %q", p)
	}
}

func TestResolveExecPrompt_Empty(t *testing.T) {
	_, err := resolveExecPrompt([]string{})
	if err == nil {
		t.Error("expected error for empty args")
	}
}

func TestPersistExecSession(t *testing.T) {
	// Set up temp session dir
	dir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)
	os.MkdirAll(filepath.Join(dir, ".hawk", "sessions"), 0o755)

	persistExecSession("test-123", "claude-opus", "anthropic", "hello", "world")

	// Check file exists
	path := filepath.Join(dir, ".hawk", "sessions", "test-123.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file not created: %v", err)
	}
}

func TestExecResult_JSON(t *testing.T) {
	r := ExecResult{
		SessionID:  "exec-123",
		Response:   "done",
		ExitCode:   0,
		TokensIn:   100,
		TokensOut:  50,
		TurnsTaken: 2,
		Duration:   "1.5s",
		Model:      "test-model",
		Worktree:   "/tmp/wt",
		Branch:     "hawk-exec/123",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ExecResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SessionID != "exec-123" {
		t.Errorf("expected exec-123, got %s", decoded.SessionID)
	}
	if decoded.Worktree != "/tmp/wt" {
		t.Errorf("expected worktree path, got %s", decoded.Worktree)
	}
	if decoded.Branch != "hawk-exec/123" {
		t.Errorf("expected branch, got %s", decoded.Branch)
	}
}
