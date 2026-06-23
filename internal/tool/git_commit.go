package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var lastAutoCommitHash string

// CommitMessageChatFn is the seam through which the LLM-based Conventional
// Commits generator reaches a fast model: it takes a prompt and returns the
// model's raw response. It is reused as the ChatFn of the existing
// CommitMessageGenerator (see smart_commit.go).
//
// Hawk's tools do not yet hold a direct handle to the eyrie model client, so
// the default value is nil and GenerateCommitMessage falls back to the
// deterministic rule-based generator. Assign this at startup once the session
// model client is threaded through the tool layer to enable real LLM output.
//
// TODO(commit-llm): wire an eyrie-backed function into CommitMessageChatFn when
// the session model client is available to tools.
var CommitMessageChatFn func(ctx context.Context, prompt string) (string, error)

// errNoCommitModel signals that no real model is wired up. Retained for callers
// and tests that want to distinguish the no-model case.
var errNoCommitModel = fmt.Errorf("no commit message model configured")

// AttributionModes carries the three independently-toggleable attribution
// channels for a commit. They build on top of the existing TrailerStyle: a
// non-empty Author/Committer override the corresponding git identity for the
// commit, while CoAuthoredBy appends a Co-authored-by trailer regardless of
// TrailerStyle.
//
// Each field is independent: enabling CoAuthoredBy does not require an Author
// override, and setting an Author override does not imply a committer change.
type AttributionModes struct {
	// Author, when non-empty, sets the commit's author identity in
	// "Name <email>" form via GIT_AUTHOR_* environment variables.
	Author string
	// Committer, when non-empty, sets the commit's committer identity in
	// "Name <email>" form via GIT_COMMITTER_* environment variables.
	Committer string
	// CoAuthoredBy, when non-empty, appends a "Co-authored-by: <value>"
	// trailer in "Name <email>" form.
	CoAuthoredBy string
}

// IsGitRepo checks if the current directory is inside a git repository.
func IsGitRepo() bool {
	return exec.CommandContext(context.Background(), "git", "rev-parse", "--git-dir").Run() == nil
}

func autoCommitEnabled(ctx context.Context) bool {
	tc := GetToolContext(ctx)
	if tc == nil {
		return false
	}
	return tc.AutoCommit
}

func AutoCommit(ctx context.Context, path, toolName, description string) error {
	if !IsGitRepo() {
		return fmt.Errorf("not a git repository")
	}

	add := exec.CommandContext(context.Background(), "git", "add", "--", path)
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	base := filepath.Base(path)
	msg := fmt.Sprintf("hawk: %s %s — %s", toolName, base, description)

	if tc := GetToolContext(ctx); tc != nil && tc.Attribution != nil {
		attr := tc.Attribution
		switch attr.TrailerStyle {
		case "assisted-by":
			msg += "\n\nAssisted-by: Hawk <hawk@graycode.ai>"
		case "none", "":
		default:
			// co-authored-by and unknown styles are treated as none
		}
		if attr.GeneratedWith {
			msg += "\nGenerated-with: Hawk"
		}
	}

	commit := exec.CommandContext(context.Background(), "git", "commit", "-m", msg)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	hash, err := gitHeadHash()
	if err == nil {
		lastAutoCommitHash = hash
	}
	return nil
}

func RevertLastAutoCommit() error {
	if lastAutoCommitHash == "" {
		return fmt.Errorf("no auto-commit to revert")
	}
	reset := exec.CommandContext(context.Background(), "git", "reset", "--soft", "HEAD~1")
	if out, err := reset.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	unstage := exec.CommandContext(context.Background(), "git", "restore", "--staged", ".")
	if out, err := unstage.CombinedOutput(); err != nil {
		return fmt.Errorf("git restore: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

func LastAutoCommitHash() string {
	return lastAutoCommitHash
}

func gitHeadHash() (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "rev-parse", "--short", "HEAD").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitHeadMessage() (string, error) {
	out, err := exec.CommandContext(context.Background(), "git", "log", "-1", "--format=%s").CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// isConventionalSubject reports whether the first line of msg already looks like
// a Conventional Commit subject (e.g. "feat: ...", "fix(scope): ...",
// "refactor!: ..."). It handles an optional "(scope)" and a trailing "!"
// breaking-change marker, and accepts "revert" in addition to validCommitTypes.
func isConventionalSubject(msg string) bool {
	subject := msg
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		subject = msg[:idx]
	}
	subject = strings.TrimSpace(subject)
	colon := strings.IndexByte(subject, ':')
	if colon <= 0 {
		return false
	}
	typePart := subject[:colon]
	// Strip an optional "(scope)" and a trailing "!" (breaking change marker).
	if paren := strings.IndexByte(typePart, '('); paren >= 0 {
		typePart = typePart[:paren]
	}
	typePart = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(typePart, "!")))
	return typePart == "revert" || isValidCommitType(typePart)
}

// GenerateCommitMessage produces a Conventional Commits message for the given
// diff and goal. It delegates to the existing CommitMessageGenerator, using a
// ToolContext-scoped commit chat function when available, then the package-level
// CommitMessageChatFn seam, and finally the deterministic rule-based generator.
//
// The returned message is normalized: if the output does not already begin with
// a Conventional Commits type prefix, a "chore: " prefix is added.
func GenerateCommitMessage(ctx context.Context, diff, goal string) (string, error) {
	chatFn := CommitMessageChatFn
	if tc := GetToolContext(ctx); tc != nil && tc.CommitMessageChatFn != nil {
		chatFn = tc.CommitMessageChatFn
	}
	gen := &CommitMessageGenerator{
		ChatFn:                 chatFn,
		FallbackToConventional: true,
		Style:                  "conventional",
		IncludeBody:            true,
	}
	msg, err := gen.GenerateMessage(ctx, CommitContext{
		Diff:             diff,
		ConversationGoal: goal,
	})
	if err != nil {
		return "", err
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "", fmt.Errorf("commit message generation produced empty output")
	}
	if !isConventionalSubject(msg) {
		msg = "chore: " + msg
	}
	return msg, nil
}

// CommitStaged commits whatever is currently staged with the given message and
// attribution modes. Author/Committer identity overrides are applied via the
// commit environment; a Co-authored-by trailer is appended to the message when
// modes.CoAuthoredBy is set. modes may be nil.
//
// This is the LLM/Conventional-Commits counterpart to AutoCommit (which stages
// a single path and uses the deterministic template). Callers that want a
// generated message should pair it with GenerateCommitMessage.
func CommitStaged(ctx context.Context, message string, modes *AttributionModes) error {
	if !IsGitRepo() {
		return fmt.Errorf("not a git repository")
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("commit message is empty")
	}
	message = applyAttributionModes(message, modes)

	commit := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commit.Env = commitEnv(os.Environ(), modes)
	if out, err := commit.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	if hash, err := gitHeadHash(); err == nil {
		lastAutoCommitHash = hash
	}
	return nil
}

// applyAttributionModes appends a Co-authored-by trailer to msg when requested.
// Author/Committer overrides are applied to the commit environment, not the
// message body, and are handled by commitEnv.
func applyAttributionModes(msg string, modes *AttributionModes) string {
	if modes == nil {
		return msg
	}
	if co := strings.TrimSpace(modes.CoAuthoredBy); co != "" {
		msg += "\n\nCo-authored-by: " + co
	}
	return msg
}

// commitEnv returns the environment for a git commit, layering Author/Committer
// identity overrides from modes on top of the current process environment. A
// nil modes (or empty fields) leaves git's normal identity resolution intact.
func commitEnv(base []string, modes *AttributionModes) []string {
	if modes == nil {
		return base
	}
	env := append([]string(nil), base...)
	if name, email, ok := splitIdentity(modes.Author); ok {
		env = append(env, "GIT_AUTHOR_NAME="+name, "GIT_AUTHOR_EMAIL="+email)
	}
	if name, email, ok := splitIdentity(modes.Committer); ok {
		env = append(env, "GIT_COMMITTER_NAME="+name, "GIT_COMMITTER_EMAIL="+email)
	}
	return env
}

// splitIdentity parses a "Name <email>" identity string. ok is false when the
// input is empty or not in the expected form.
func splitIdentity(identity string) (name, email string, ok bool) {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return "", "", false
	}
	open := strings.LastIndexByte(identity, '<')
	close := strings.LastIndexByte(identity, '>')
	if open < 0 || close < open {
		return "", "", false
	}
	name = strings.TrimSpace(identity[:open])
	email = strings.TrimSpace(identity[open+1 : close])
	if email == "" {
		return "", "", false
	}
	return name, email, true
}
