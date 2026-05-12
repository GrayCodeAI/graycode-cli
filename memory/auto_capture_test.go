package memory

import (
	"testing"
	"time"
)

func TestExtractConventions(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"Always use strict mode in TypeScript", 1},
		{"We use jest for testing", 1},
		{"Never commit directly to main", 1},
		{"The convention is to use named exports", 1},
		{"Prefer functional components over class components", 1},
		{"Just a normal sentence without any patterns", 0},
		{"Make sure to run tests before committing", 1},
	}

	for _, tt := range tests {
		got := ExtractConventions(tt.input)
		if len(got) != tt.want {
			t.Errorf("ExtractConventions(%q) = %d results, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestCanonicalName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"write", "Write"},
		{"file_write", "Write"},
		{"Edit", "Edit"},
		{"file_edit", "Edit"},
		{"bash", "Bash"},
		{"shell", "Bash"},
		{"read", "Read"},
		{"grep", "Grep"},
		{"unknown_tool", "unknown_tool"},
	}

	for _, tt := range tests {
		got := canonicalName(tt.input)
		if got != tt.want {
			t.Errorf("canonicalName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPatternMatchers(t *testing.T) {
	if !isTestCommand("go test ./...") {
		t.Error("expected 'go test ./...' to be detected as test command")
	}
	if !isTestCommand("npm test") {
		t.Error("expected 'npm test' to be detected as test command")
	}
	if isTestCommand("go build .") {
		t.Error("expected 'go build .' NOT to be detected as test command")
	}

	if !isGitCommit("git commit -m 'fix bug'") {
		t.Error("expected git commit to be detected")
	}
	if isGitCommit("git status") {
		t.Error("expected git status NOT to be detected as commit")
	}

	if !isPackageInstall("npm install lodash") {
		t.Error("expected npm install to be detected")
	}
	if !isPackageInstall("go get github.com/foo/bar") {
		t.Error("expected go get to be detected")
	}

	if !isBuildCommand("go build .") {
		t.Error("expected go build to be detected")
	}
	if !isBuildCommand("docker build -t app .") {
		t.Error("expected docker build to be detected")
	}
}

func TestExtractCommitMessage(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{`git commit -m "fix: resolve auth bug"`, "fix: resolve auth bug"},
		{`git commit -m 'add new feature'`, "add new feature"},
		{`git commit --amend`, ""},
	}

	for _, tt := range tests {
		got := extractCommitMessage(tt.cmd)
		if got != tt.want {
			t.Errorf("extractCommitMessage(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate short string = %q", got)
	}
	if got := truncate("this is a longer string", 10); got != "this is a ..." {
		t.Errorf("truncate long string = %q", got)
	}
}

func TestAutoCaptureMetrics(t *testing.T) {
	m := &CaptureMetrics{}
	m.inc("convention")
	m.inc("convention")
	m.inc("bug")

	if m.Captured != 3 {
		t.Errorf("Captured = %d, want 3", m.Captured)
	}
	if m.ConventionsOut != 2 {
		t.Errorf("ConventionsOut = %d, want 2", m.ConventionsOut)
	}
	if m.BugsOut != 1 {
		t.Errorf("BugsOut = %d, want 1", m.BugsOut)
	}
}

func TestAutoCaptureWithNilBridge(t *testing.T) {
	bridge := &YaadBridge{ready: false}
	ac := NewAutoCapture(bridge)
	defer ac.Stop()

	// Should not panic
	ac.Ingest("Write", map[string]interface{}{"file_path": "/tmp/test.go"}, "ok", false)
	time.Sleep(10 * time.Millisecond)

	m := ac.Metrics()
	if m.Captured != 0 {
		t.Errorf("expected no captures when bridge not ready, got %d", m.Captured)
	}
}
