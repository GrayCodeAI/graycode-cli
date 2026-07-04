package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// --- Skill dispatch tests ---------------------------------------------------

// mockSkillRunner is a test double for skillRunner.
type mockSkillRunner struct {
	called string
	out    string
	err    error
}

func (m *mockSkillRunner) Run(name string) (string, error) {
	m.called = name
	if m.err != nil {
		return "", m.err
	}
	if m.out != "" {
		return m.out, nil
	}
	return "rendered:" + name, nil
}

func TestIsSkillDispatch(t *testing.T) {
	cases := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"slash skill", "/deep-research climate", true},
		{"slash only name", "/verify", true},
		{"plain prompt", "fix the tests", false},
		{"empty", "", false},
		{"lone slash", "/", false},
		{"leading space then slash", "  /review", true},
		{"comment double slash", "// not a skill", false},
		{"path-like prompt", "edit /etc/hosts", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSkillDispatch(tc.prompt); got != tc.want {
				t.Errorf("isSkillDispatch(%q) = %v, want %v", tc.prompt, got, tc.want)
			}
		})
	}
}

func TestParseSkillInvocation(t *testing.T) {
	cases := []struct {
		prompt   string
		wantName string
		wantArgs string
	}{
		{"/deep-research climate change", "deep-research", "climate change"},
		{"/verify", "verify", ""},
		{"  /review  the diff ", "review", "the diff"},
		{"/code-review --high", "code-review", "--high"},
	}
	for _, tc := range cases {
		t.Run(tc.prompt, func(t *testing.T) {
			name, args := parseSkillInvocation(tc.prompt)
			if name != tc.wantName || args != tc.wantArgs {
				t.Errorf("parseSkillInvocation(%q) = (%q, %q), want (%q, %q)",
					tc.prompt, name, args, tc.wantName, tc.wantArgs)
			}
		})
	}
}

func TestDispatchSkill_RoutesToRunner(t *testing.T) {
	runner := &mockSkillRunner{out: "SKILL BODY"}
	out, err := dispatchSkill(runner, "/deep-research find sources")
	if err != nil {
		t.Fatal(err)
	}
	if runner.called != "deep-research" {
		t.Errorf("runner called with %q, want deep-research", runner.called)
	}
	if want := "SKILL BODY\n\nArguments: find sources"; out != want {
		t.Errorf("dispatchSkill output = %q, want %q", out, want)
	}
}

func TestDispatchSkill_NoArgs(t *testing.T) {
	runner := &mockSkillRunner{out: "BODY"}
	out, err := dispatchSkill(runner, "/verify")
	if err != nil {
		t.Fatal(err)
	}
	if out != "BODY" {
		t.Errorf("output = %q, want BODY (no args appended)", out)
	}
}

func TestDispatchSkill_RunnerError(t *testing.T) {
	runner := &mockSkillRunner{err: fmt.Errorf("skill \"ghost\" not found")}
	if _, err := dispatchSkill(runner, "/ghost"); err == nil {
		t.Error("expected error when runner fails")
	}
}

// --- GitHub Actions detection tests -----------------------------------------

func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func fileFunc(payload string) func(string) ([]byte, error) {
	return func(string) ([]byte, error) { return []byte(payload), nil }
}

func TestDetectGitHubActions_NotActive(t *testing.T) {
	gha := detectGitHubActions(envFunc(map[string]string{}), fileFunc(""))
	if gha.Active {
		t.Error("expected Active=false when GITHUB_ACTIONS unset")
	}
	if gha.Mode != GHAModeNone {
		t.Errorf("expected GHAModeNone, got %q", gha.Mode)
	}
}

func TestDetectGitHubActions_InteractiveMention(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "issue_comment",
		"GITHUB_EVENT_PATH": "/tmp/event.json",
	}
	payload := `{"comment":{"body":"@hawk please fix the failing test"}}`
	gha := detectGitHubActions(envFunc(env), fileFunc(payload))
	if !gha.Active {
		t.Fatal("expected Active=true")
	}
	if gha.Mode != GHAModeInteractive {
		t.Errorf("expected interactive mode, got %q", gha.Mode)
	}
	if !gha.Mention {
		t.Error("expected Mention=true")
	}
	if gha.Prompt != "please fix the failing test" {
		t.Errorf("prompt = %q, want mention stripped", gha.Prompt)
	}
}

func TestDetectGitHubActions_ReviewCommentMention(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "pull_request_review_comment",
		"GITHUB_EVENT_PATH": "/tmp/event.json",
	}
	payload := `{"comment":{"body":"@Hawk explain this change"}}`
	gha := detectGitHubActions(envFunc(env), fileFunc(payload))
	if gha.Mode != GHAModeInteractive {
		t.Errorf("expected interactive mode for review comment, got %q", gha.Mode)
	}
	if gha.Prompt != "explain this change" {
		t.Errorf("prompt = %q", gha.Prompt)
	}
}

func TestDetectGitHubActions_AutomationIssue(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "issues",
		"GITHUB_EVENT_PATH": "/tmp/event.json",
	}
	payload := `{"issue":{"title":"Bug: crash on save","body":"steps to reproduce..."}}`
	gha := detectGitHubActions(envFunc(env), fileFunc(payload))
	if gha.Mode != GHAModeAutomation {
		t.Errorf("expected automation mode, got %q", gha.Mode)
	}
	if gha.Prompt != "Bug: crash on save\n\nsteps to reproduce..." {
		t.Errorf("prompt = %q", gha.Prompt)
	}
}

func TestDetectGitHubActions_CommentNoMentionIsAutomation(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "issue_comment",
		"GITHUB_EVENT_PATH": "/tmp/event.json",
	}
	payload := `{"comment":{"body":"just a normal comment"}}`
	gha := detectGitHubActions(envFunc(env), fileFunc(payload))
	if gha.Mode != GHAModeAutomation {
		t.Errorf("expected automation mode for non-mention comment, got %q", gha.Mode)
	}
	if gha.Mention {
		t.Error("expected Mention=false")
	}
}

func TestDetectGitHubActions_TrustFromAssociation(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":    "true",
		"GITHUB_EVENT_NAME": "issues",
		"GITHUB_EVENT_PATH": "/tmp/event.json",
	}
	// Insider association → trusted.
	owner := `{"issue":{"title":"t","body":"b","author_association":"OWNER"}}`
	if gha := detectGitHubActions(envFunc(env), fileFunc(owner)); !gha.Trusted {
		t.Errorf("OWNER should be trusted, got association=%q trusted=%v", gha.AuthorAssociation, gha.Trusted)
	}
	// Outside contributor → untrusted.
	outsider := `{"issue":{"title":"t","body":"b","author_association":"NONE"}}`
	if gha := detectGitHubActions(envFunc(env), fileFunc(outsider)); gha.Trusted {
		t.Error("NONE association should be untrusted")
	}
	// Missing association → untrusted (fail closed).
	none := `{"issue":{"title":"t","body":"b"}}`
	if gha := detectGitHubActions(envFunc(env), fileFunc(none)); gha.Trusted {
		t.Error("missing association should be untrusted")
	}
}

func TestDetectGitHubActions_TrustEnvOverride(t *testing.T) {
	env := map[string]string{
		"GITHUB_ACTIONS":       "true",
		"GITHUB_EVENT_NAME":    "issues",
		"GITHUB_EVENT_PATH":    "/tmp/event.json",
		"HAWK_GHA_TRUST_EVENT": "1",
	}
	outsider := `{"issue":{"title":"t","body":"b","author_association":"NONE"}}`
	if gha := detectGitHubActions(envFunc(env), fileFunc(outsider)); !gha.Trusted {
		t.Error("HAWK_GHA_TRUST_EVENT=1 should trust even NONE association")
	}
}

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
	t.Setenv("HAWK_STATE_DIR", filepath.Join(dir, "state"))

	persistExecSession("test-123", "claude-opus", "anthropic", "hello", "world")

	// Check file exists
	path := filepath.Join(storage.SessionsDir(), "test-123.jsonl")
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
