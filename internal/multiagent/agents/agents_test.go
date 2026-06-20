package agents

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleAgent = `---
name: reviewer
description: Code review specialist focused on security
model: claude-sonnet-4-6
---
# Code Reviewer

You are a security-focused code reviewer. Analyze diffs for vulnerabilities.

## Guidelines
- Check for injection attacks
- Verify input validation
- Flag hardcoded secrets
`

func TestParse(t *testing.T) {
	agent, err := Parse(sampleAgent, "reviewer.md")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if agent.Name != "reviewer" {
		t.Errorf("expected name 'reviewer', got %q", agent.Name)
	}
	if agent.Description != "Code review specialist focused on security" {
		t.Errorf("unexpected description: %q", agent.Description)
	}
	if agent.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model 'claude-sonnet-4-6', got %q", agent.Model)
	}
	if agent.Prompt == "" {
		t.Error("expected non-empty prompt body")
	}
	if agent.Prompt[0] != '#' {
		t.Errorf("prompt should start with markdown heading, got: %q", agent.Prompt[:20])
	}
}

func TestParse_InheritModel(t *testing.T) {
	content := "---\nname: worker\nmodel: inherit\n---\nDo work."
	agent, err := Parse(content, "worker.md")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Model != "" {
		t.Errorf("inherit should result in empty model, got %q", agent.Model)
	}
}

func TestParse_NoFrontmatter(t *testing.T) {
	_, err := Parse("just a prompt", "test.md")
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParse_NameFromFilename(t *testing.T) {
	content := "---\ndescription: test\n---\nPrompt body"
	agent, err := Parse(content, "/path/to/my-agent.md")
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "my-agent" {
		t.Errorf("expected name derived from filename, got %q", agent.Name)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-agent.md")
	os.WriteFile(path, []byte(sampleAgent), 0o644)

	agent, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if agent.Name != "reviewer" {
		t.Errorf("expected 'reviewer', got %q", agent.Name)
	}
}

func TestListAll_FromDir(t *testing.T) {
	dir := t.TempDir()

	// Write some agent files
	if err := os.WriteFile(filepath.Join(dir, "worker.md"), []byte("---\nname: worker\n---\nDo work."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("---\nname: reviewer\n---\nReview code."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-md.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Override the agent dirs for testing
	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatal(err)
	}
	defer os.Setenv("HOME", origHome) //nolint:errcheck

	stateDir := filepath.Join(dir, "state")
	t.Setenv("HAWK_STATE_DIR", stateDir)
	agentDir := filepath.Join(stateDir, "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "test.md"), []byte("---\nname: test\n---\nTest prompt."), 0o644); err != nil {
		t.Fatal(err)
	}

	agents, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(agents) < 1 {
		t.Error("expected at least 1 agent from user state agents dir")
	}
}

func TestGet_NotFound(t *testing.T) {
	_, err := Get("nonexistent-agent-xyz")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}
