package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

// setupTestRepo creates a temporary git repo and changes into it.
// The caller should defer the returned cleanup function.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v failed: %s (%v)", args, out, err)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	// Create an initial commit so HEAD exists.
	initial := filepath.Join(dir, "README")
	os.WriteFile(initial, []byte("init"), 0o644)
	run("git", "add", "README")
	run("git", "commit", "-m", "initial commit")

	return dir, func() { os.Chdir(origDir) }
}

func TestIsGitRepo(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()
	_ = dir

	if !IsGitRepo() {
		t.Fatal("expected IsGitRepo() == true inside test repo")
	}
}

func TestAutoCommitAndRevert(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create a file and auto-commit it.
	file := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AutoCommit(context.Background(), file, "Write", "wrote file"); err != nil {
		t.Fatalf("AutoCommit: %v", err)
	}

	hash := LastAutoCommitHash()
	if hash == "" {
		t.Fatal("expected non-empty LastAutoCommitHash()")
	}

	// Verify the commit message.
	msg, err := gitHeadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(msg, "graycode: Write hello.txt") {
		t.Fatalf("unexpected commit message: %q", msg)
	}

	// Revert the auto-commit.
	if revertErr := RevertLastAutoCommit(); revertErr != nil {
		t.Fatalf("RevertLastAutoCommit: %v", revertErr)
	}

	// After revert, HEAD should point to the commit before auto-commit.
	msg2, err := gitHeadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg2 != "initial commit" {
		t.Fatalf("expected revert to initial commit, got: %q", msg2)
	}
}

func TestRevertNonGraycodeCommitFails(t *testing.T) {
	_, cleanup := setupTestRepo(t)
	defer cleanup()

	// HEAD is "initial commit" — not a graycode commit.
	if err := RevertLastAutoCommit(); err == nil {
		t.Fatal("expected error reverting non-graycode commit")
	}
}

func TestAutoCommitOutsideGitRepo(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	err := AutoCommit(context.Background(), "/tmp/nonexistent", "Write", "test")
	if err == nil {
		t.Fatal("expected error outside git repo")
	}
}

func gitCommitBody(t *testing.T) string {
	t.Helper()
	out, err := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=%B").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return string(out)
}

func TestAutoCommitOmitsCoAuthorTrailer(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	file := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := WithToolContext(context.Background(), &ToolContext{
		AutoCommit: true,
		Attribution: &types.Attribution{
			TrailerStyle: "co-authored-by",
		},
	})
	if err := AutoCommit(ctx, file, "Write", "wrote file"); err != nil {
		t.Fatalf("AutoCommit: %v", err)
	}

	body := gitCommitBody(t)
	if strings.Contains(strings.ToLower(body), "co-authored-by:") {
		t.Fatalf("unexpected co-author trailer in commit body: %q", body)
	}
}

func TestAutoCommitAssistedByTrailerOptional(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	file := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := WithToolContext(context.Background(), &ToolContext{
		AutoCommit: true,
		Attribution: &types.Attribution{
			TrailerStyle: "assisted-by",
		},
	})
	if err := AutoCommit(ctx, file, "Write", "wrote file"); err != nil {
		t.Fatalf("AutoCommit: %v", err)
	}

	body := gitCommitBody(t)
	if !strings.Contains(body, "Assisted-by: Graycode") {
		t.Fatalf("expected assisted-by trailer, got: %q", body)
	}
}

func TestGenerateCommitMessageWithMockModel(t *testing.T) {
	orig := CommitMessageChatFn
	defer func() { CommitMessageChatFn = orig }()

	tests := []struct {
		name       string
		modelReply string
		modelErr   bool
		wantPrefix string
	}{
		{
			name:       "already conventional feat",
			modelReply: "feat: add SQL exploration tool",
			wantPrefix: "feat:",
		},
		{
			name:       "already conventional fix with scope",
			modelReply: "fix(tool): handle nil rows",
			wantPrefix: "fix(tool):",
		},
		{
			// The underlying generator's response parser already coerces
			// free-form replies into a conventional subject, so we only assert
			// the result is conventional (not a specific prefix).
			name:       "non-conventional reply is conventionalized",
			modelReply: "wire up the database connection",
			wantPrefix: "",
		},
		{
			// On model error the generator falls back to rule-based output,
			// which is always conventional. We only assert it has a type prefix.
			name:       "model error falls back to rule-based",
			modelErr:   true,
			modelReply: "",
			wantPrefix: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			CommitMessageChatFn = func(_ context.Context, _ string) (string, error) {
				if tc.modelErr {
					return "", errNoCommitModel
				}
				return tc.modelReply, nil
			}

			msg, err := GenerateCommitMessage(context.Background(), "diff --git a/x b/x\n+hello", "add a thing")
			if err != nil {
				t.Fatalf("GenerateCommitMessage: %v", err)
			}
			if msg == "" {
				t.Fatal("expected non-empty message")
			}
			if !isConventionalSubject(msg) {
				t.Fatalf("expected conventional subject, got %q", msg)
			}
			if tc.wantPrefix != "" && !strings.HasPrefix(msg, tc.wantPrefix) {
				t.Fatalf("expected prefix %q, got %q", tc.wantPrefix, msg)
			}
		})
	}
}

func TestGenerateCommitMessagePrefersContextChatFn(t *testing.T) {
	orig := CommitMessageChatFn
	defer func() { CommitMessageChatFn = orig }()

	CommitMessageChatFn = func(_ context.Context, _ string) (string, error) {
		return "fix: global should not be used", nil
	}
	ctx := WithToolContext(context.Background(), &ToolContext{
		CommitMessageChatFn: func(_ context.Context, _ string) (string, error) {
			return "feat: use context commit model", nil
		},
	})

	msg, err := GenerateCommitMessage(ctx, "diff --git a/x b/x\n+hello", "add a thing")
	if err != nil {
		t.Fatalf("GenerateCommitMessage: %v", err)
	}
	if !strings.HasPrefix(msg, "feat: use context commit model") {
		t.Fatalf("expected context chat function result, got %q", msg)
	}
}

func TestGenerateCommitMessageFallsBackWithoutChatFn(t *testing.T) {
	orig := CommitMessageChatFn
	defer func() { CommitMessageChatFn = orig }()
	CommitMessageChatFn = nil

	msg, err := GenerateCommitMessage(context.Background(), "diff --git a/README.md b/README.md\n+docs", "update docs")
	if err != nil {
		t.Fatalf("GenerateCommitMessage: %v", err)
	}
	if msg == "" || !isConventionalSubject(msg) {
		t.Fatalf("expected conventional fallback message, got %q", msg)
	}
}

func TestIsConventionalSubject(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"feat: add thing", true},
		{"fix(scope): bug", true},
		{"refactor!: breaking", true},
		{"revert: undo it", true},
		{"chore: deps\n\nbody text", true},
		{"just a plain message", false},
		{"WIP: not a type", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isConventionalSubject(tc.msg); got != tc.want {
			t.Errorf("isConventionalSubject(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}

func TestAttributionModesIndependent(t *testing.T) {
	tests := []struct {
		name             string
		modes            *AttributionModes
		wantCoAuthor     bool
		wantAuthorEnv    bool
		wantCommitterEnv bool
	}{
		{
			name:  "nil modes: nothing applied",
			modes: nil,
		},
		{
			name:         "co-authored-by only",
			modes:        &AttributionModes{CoAuthoredBy: "Graycode <graycode@graycode.ai>"},
			wantCoAuthor: true,
		},
		{
			name:          "author only",
			modes:         &AttributionModes{Author: "Alice <alice@example.com>"},
			wantAuthorEnv: true,
		},
		{
			name:             "committer only",
			modes:            &AttributionModes{Committer: "Bob <bob@example.com>"},
			wantCommitterEnv: true,
		},
		{
			name: "all three independently",
			modes: &AttributionModes{
				Author:       "Alice <alice@example.com>",
				Committer:    "Bob <bob@example.com>",
				CoAuthoredBy: "Graycode <graycode@graycode.ai>",
			},
			wantCoAuthor:     true,
			wantAuthorEnv:    true,
			wantCommitterEnv: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Co-authored-by trailer is applied to the message body.
			msg := applyAttributionModes("feat: x", tc.modes)
			hasCo := strings.Contains(msg, "Co-authored-by: Graycode <graycode@graycode.ai>")
			if hasCo != tc.wantCoAuthor {
				t.Errorf("co-author trailer present=%v, want %v (msg=%q)", hasCo, tc.wantCoAuthor, msg)
			}

			// Author/Committer overrides are applied to the env, not the body.
			env := commitEnv(nil, tc.modes)
			hasAuthor := envHasKey(env, "GIT_AUTHOR_NAME")
			hasCommitter := envHasKey(env, "GIT_COMMITTER_NAME")
			if hasAuthor != tc.wantAuthorEnv {
				t.Errorf("GIT_AUTHOR_NAME present=%v, want %v", hasAuthor, tc.wantAuthorEnv)
			}
			if hasCommitter != tc.wantCommitterEnv {
				t.Errorf("GIT_COMMITTER_NAME present=%v, want %v", hasCommitter, tc.wantCommitterEnv)
			}
		})
	}
}

func envHasKey(env []string, key string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return true
		}
	}
	return false
}

func TestCommitStagedWithAttribution(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	file := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(context.Background(), "git", "add", "x.txt").CombinedOutput(); err != nil {
		t.Fatalf("git add: %s (%v)", out, err)
	}

	modes := &AttributionModes{
		Author:       "Alice <alice@example.com>",
		CoAuthoredBy: "Graycode <graycode@graycode.ai>",
	}
	if err := CommitStaged(context.Background(), "feat: add x", modes); err != nil {
		t.Fatalf("CommitStaged: %v", err)
	}

	body := gitCommitBody(t)
	if !strings.Contains(body, "Co-authored-by: Graycode <graycode@graycode.ai>") {
		t.Fatalf("expected co-author trailer, got: %q", body)
	}

	authorEmail, err := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=%ae").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(authorEmail)) != "alice@example.com" {
		t.Fatalf("expected author override, got: %q", authorEmail)
	}
	// Committer should NOT have been overridden (independent toggles).
	committerEmail, err := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=%ce").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(committerEmail)) == "alice@example.com" {
		t.Fatal("committer should not have been overridden when only Author is set")
	}
}
