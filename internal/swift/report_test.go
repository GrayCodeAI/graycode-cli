package swift

import (
	"strings"
	"testing"
	"time"
)

func sampleSnapshot() *Snapshot {
	return &Snapshot{
		Timestamp:      time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
		Version:        "v1.0.0",
		GitCommit:      "abc1234",
		Build:          "release",
		Platform:       "darwin/arm64",
		Model:          "claude-sonnet",
		PermissionMode: "default",
		Sandbox:        "default",
		Workspace:      "/Users/me/repo",
		SessionID:      "sess-01",
		SessionDir:     "/tmp/graycode-sess-01",
		PID:            4242,
		Terminal:       "120x30",
		Env:            []string{"HOME=/Users/me", "HOST=box", "API_TOKEN=sk-ant-abcdefABCDEF1234"},
		StableRules: []StableRule{
			{ID: 1, Kind: "command", Identity: "ls -la", Decision: "allow"},
			{ID: 2, Kind: "structured_tool", Identity: "/secrets/key=sk-abcdef123456", Decision: "deny", Sensitive: true},
		},
		PermissionGrants: []string{"shell:allow:.*"},
		Activity: []Activity{
			{Timestamp: time.Date(2026, 8, 21, 11, 59, 30, 0, time.UTC), Kind: "tool", Name: "read internal/x.go", OK: true, Duration: 12 * time.Millisecond},
		},
		LogTail: []LogEntry{
			{Line: "token=sk-ant-api03-abcdefGHIJKL", Sensitive: true},
		},
		Transcript: []LogEntry{
			{Line: "\x1b[32muser\x1b[0m: hello", Sensitive: true},
		},
	}
}

func TestBuildSections(t *testing.T) {
	t.Parallel()
	out := Build(sampleSnapshot())
	for _, want := range []string{
		"# graycode swift",
		"Private diagnostic report.",
		"## Summary",
		"## Current State",
		"## Permissions",
		"## Recent Activity",
		"## Logs",
		"## Transcript",
		"generated: 2026-08-21T12:00:00Z",
		"version: v1.0.0 (abc1234)",
		"platform: darwin/arm64",
		"workspace: /Users/me/repo",
		"process: pid=4242",
		"terminal: 120x30",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Build output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "sk-ant-abcdefABCDEF1234") {
		t.Errorf("env secret leaked into output:\n%s", out)
	}
	if strings.Contains(out, "sk-abcdef123456") {
		t.Errorf("sensitive stable-rule identity leaked:\n%s", out)
	}
	if strings.Contains(out, "sk-ant-api03-abcdefGHIJKL") {
		t.Errorf("log secret leaked:\n%s", out)
	}
}

func TestBuildEmpty(t *testing.T) {
	t.Parallel()
	out := Build(nil)
	if !strings.HasPrefix(out, "# graycode swift") {
		t.Errorf("Build(nil) header missing")
	}
}

func TestBuildStripsANSIIntranscript(t *testing.T) {
	t.Parallel()
	s := sampleSnapshot()
	s.Transcript = []LogEntry{{Line: "\x1b[31mred\x1b[0m text", Sensitive: true}}
	out := Build(s)
	if strings.Contains(out, "\x1b[") {
		t.Errorf("ANSI escapes leaked into transcript output:\n%q", out)
	}
	if !strings.Contains(out, "red text") {
		t.Errorf("visible transcript text missing")
	}
}

func TestBuildCapsLogLineLength(t *testing.T) {
	t.Parallel()
	s := sampleSnapshot()
	s.Transcript = []LogEntry{{Line: strings.Repeat("x", MaxLineBytes+50)}}
	out := Build(s)
	// find the transcript line
	idx := strings.Index(out, "  "+strings.Repeat("x", MaxLineBytes))
	if idx < 0 {
		t.Fatalf("expected capped long line")
	}
	if !strings.HasSuffix(out[:strings.Index(out[idx:], "\n")+idx], " ...") {
		t.Errorf("long line not truncated with ellipsis")
	}
}
