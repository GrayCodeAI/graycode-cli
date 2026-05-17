package prompts

import (
	"os"
	"strings"
	"testing"
)

func TestGatherWorkspaceContext(t *testing.T) {
	// Use the project root which is a git repo
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	// Walk up to find the repo root (where .git lives)
	dir := cwd
	for {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			break
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Skip("not in a git repo")
		}
		dir = parent
	}

	ctx := GatherWorkspaceContext(dir)
	if ctx == nil {
		t.Fatal("GatherWorkspaceContext returned nil")
	}

	if ctx.GitBranch == "" {
		t.Error("expected non-empty GitBranch")
	}

	if len(ctx.TopFiles) == 0 {
		t.Error("expected non-empty TopFiles")
	}

	if ctx.Language == "" {
		t.Error("expected detected language for a Go project")
	}

	if ctx.Language != "Go" {
		t.Errorf("expected language 'Go', got %q", ctx.Language)
	}
}

func TestWorkspaceContextFormat(t *testing.T) {
	ctx := &WorkspaceContext{
		GitBranch:     "main",
		GitStatus:     "3 files modified",
		RecentCommits: []string{"fix: handle nil pointer", "feat: add caching", "refactor: split module"},
		TopFiles:      []string{"cmd/", "config/", "engine/", "model/", "session/", "tool/"},
		Language:      "Go",
	}

	formatted := ctx.Format()
	if !strings.Contains(formatted, "## Project Context") {
		t.Error("missing section header")
	}
	if !strings.Contains(formatted, "Branch: main") {
		t.Error("missing branch")
	}
	if !strings.Contains(formatted, "3 files modified") {
		t.Error("missing status")
	}
	if !strings.Contains(formatted, "fix: handle nil pointer") {
		t.Error("missing recent commit")
	}
	if !strings.Contains(formatted, "Go project") {
		t.Error("missing language")
	}
}

func TestWorkspaceContextFormatNil(t *testing.T) {
	var ctx *WorkspaceContext
	if ctx.Format() != "" {
		t.Error("nil WorkspaceContext.Format should return empty string")
	}
}

func TestDetectLanguage(t *testing.T) {
	// Test on the project root — should detect Go
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			break
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Skip("not in a git repo")
		}
		dir = parent
	}

	lang := detectLanguage(dir)
	if lang != "Go" {
		t.Errorf("expected 'Go', got %q", lang)
	}
}

func TestReadGitBranch(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(dir + "/.git"); err == nil {
			break
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Skip("not in a git repo")
		}
		dir = parent
	}

	branch := readGitBranch(dir)
	if branch == "" {
		t.Error("expected non-empty branch from readGitBranch")
	}
}
