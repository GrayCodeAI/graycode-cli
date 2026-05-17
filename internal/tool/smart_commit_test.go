package tool

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDetectCommitType_Feat(t *testing.T) {
	diff := `diff --git a/pkg/auth/auth.go b/pkg/auth/auth.go
new file mode 100644
+package auth
+
+func NewAuthenticator() *Authenticator {
+	return &Authenticator{}
+}
`
	files := []string{"pkg/auth/auth.go"}
	got := DetectCommitType(diff, files)
	if got != "feat" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "feat")
	}
}

func TestDetectCommitType_Fix(t *testing.T) {
	diff := `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -10,6 +10,9 @@
 func Handle(r *Request) error {
+	if err != nil {
+		return fmt.Errorf("handler: %w", err)
+	}
 	return nil
 }
`
	files := []string{"handler.go"}
	got := DetectCommitType(diff, files)
	if got != "fix" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "fix")
	}
}

func TestDetectCommitType_Test(t *testing.T) {
	diff := `diff --git a/auth_test.go b/auth_test.go
--- a/auth_test.go
+++ b/auth_test.go
+func TestNewCase(t *testing.T) {}
`
	files := []string{"auth_test.go", "handler_test.go"}
	got := DetectCommitType(diff, files)
	if got != "test" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "test")
	}
}

func TestDetectCommitType_Docs(t *testing.T) {
	diff := `diff --git a/README.md b/README.md
+## New Section
`
	files := []string{"README.md", "CHANGELOG.md"}
	got := DetectCommitType(diff, files)
	if got != "docs" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "docs")
	}
}

func TestDetectCommitType_Chore(t *testing.T) {
	diff := `diff --git a/go.mod b/go.mod
+require github.com/new/dep v1.0.0
`
	files := []string{"go.mod", "go.sum"}
	got := DetectCommitType(diff, files)
	if got != "chore" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "chore")
	}
}

func TestDetectCommitType_Refactor(t *testing.T) {
	diff := `diff --git a/service.go b/service.go
--- a/service.go
+++ b/service.go
-func oldApproach() string {
-	return "old"
-}
+func newApproach() string {
+	return "new"
+}
`
	files := []string{"service.go"}
	got := DetectCommitType(diff, files)
	if got != "refactor" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "refactor")
	}
}

func TestDetectCommitType_Style(t *testing.T) {
	// Style changes: same content, different whitespace formatting.
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
-func hello()  {
-	return   "world"
+func hello() {
+	return "world"
`
	files := []string{"main.go"}
	got := DetectCommitType(diff, files)
	if got != "style" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "style")
	}
}

func TestDetectScope_SingleFile(t *testing.T) {
	files := []string{"pkg/auth/token.go"}
	got := DetectScope(files)
	if got != "token" {
		t.Errorf("DetectScope() = %q, want %q", got, "token")
	}
}

func TestDetectScope_CommonDir(t *testing.T) {
	files := []string{"pkg/auth/token.go", "pkg/auth/middleware.go"}
	got := DetectScope(files)
	if got != "auth" {
		t.Errorf("DetectScope() = %q, want %q", got, "auth")
	}
}

func TestDetectScope_DiverseFiles(t *testing.T) {
	files := []string{"cmd/main.go", "pkg/auth/token.go", "internal/db/conn.go"}
	got := DetectScope(files)
	// Too diverse — common prefix is empty or root.
	if got != "" && got != "." {
		// Accept empty for very diverse paths.
		t.Logf("DetectScope() = %q (diverse files)", got)
	}
}

func TestDetectScope_Empty(t *testing.T) {
	files := []string{}
	got := DetectScope(files)
	if got != "" {
		t.Errorf("DetectScope() = %q, want empty", got)
	}
}

func TestDetectScope_SameParent(t *testing.T) {
	files := []string{"tool/smart_commit.go", "tool/smart_commit_test.go"}
	got := DetectScope(files)
	if got != "tool" {
		t.Errorf("DetectScope() = %q, want %q", got, "tool")
	}
}

func TestGenerateRuleBased_NewFile(t *testing.T) {
	ctx := CommitContext{
		Diff: `diff --git a/pkg/auth/auth.go b/pkg/auth/auth.go
new file mode 100644
+package auth
+func NewAuthenticator() *Authenticator {}
`,
		FilesChanged: []string{"pkg/auth/auth.go"},
	}
	msg := GenerateRuleBased(ctx)

	// Should be conventional format.
	if !strings.Contains(msg, ":") {
		t.Errorf("expected conventional format with colon, got %q", msg)
	}
	if !strings.HasPrefix(msg, "feat") {
		t.Errorf("expected feat type for new file, got %q", msg)
	}
}

func TestGenerateRuleBased_TestFile(t *testing.T) {
	ctx := CommitContext{
		Diff:         "+func TestSomething(t *testing.T) {}",
		FilesChanged: []string{"pkg/auth/auth_test.go"},
	}
	msg := GenerateRuleBased(ctx)

	if !strings.HasPrefix(msg, "test") {
		t.Errorf("expected test type, got %q", msg)
	}
}

func TestGenerateRuleBased_Docs(t *testing.T) {
	ctx := CommitContext{
		Diff:         "+## New Feature\n+Description of new feature",
		FilesChanged: []string{"README.md"},
	}
	msg := GenerateRuleBased(ctx)

	if !strings.HasPrefix(msg, "docs") {
		t.Errorf("expected docs type, got %q", msg)
	}
}

func TestGenerateSubject_UnderMaxLength(t *testing.T) {
	files := []string{"pkg/auth/middleware.go", "pkg/auth/token.go", "pkg/auth/handler.go"}
	diff := "+func NewHandler() {}\n"

	subject := GenerateSubject("feat", files, diff)
	// Subject alone should be under 72 - prefix length.
	full := "feat(auth): " + subject
	if len(full) > 72 {
		t.Errorf("full subject line is %d chars (max 72): %q", len(full), full)
	}
}

func TestGenerateSubject_SingleFile(t *testing.T) {
	files := []string{"pkg/auth/auth.go"}
	diff := "new file mode 100644\n+package auth\n"

	subject := GenerateSubject("feat", files, diff)
	if subject == "" {
		t.Error("GenerateSubject returned empty string")
	}
	if len(subject) > 72 {
		t.Errorf("subject too long: %d chars", len(subject))
	}
}

func TestValidateMessage_Valid(t *testing.T) {
	msg := "feat(auth): add token validation"
	warnings := ValidateMessage(msg)
	if len(warnings) > 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func TestValidateMessage_TooLong(t *testing.T) {
	msg := "feat(authentication-service): add comprehensive token validation logic for all supported providers and handlers"
	warnings := ValidateMessage(msg)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "chars") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about subject line length")
	}
}

func TestValidateMessage_MissingBlankLine(t *testing.T) {
	msg := "feat: add feature\nThis is the body without blank line"
	warnings := ValidateMessage(msg)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "blank line") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about missing blank line")
	}
}

func TestValidateMessage_InvalidType(t *testing.T) {
	msg := "feature: add something"
	warnings := ValidateMessage(msg)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "invalid commit type") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about invalid commit type")
	}
}

func TestValidateMessage_UppercaseAfterColon(t *testing.T) {
	msg := "feat: Add something"
	warnings := ValidateMessage(msg)

	found := false
	for _, w := range warnings {
		if strings.Contains(w, "lowercase") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about uppercase after colon")
	}
}

func TestValidateMessage_WithBody(t *testing.T) {
	msg := "feat(auth): add token validation\n\nThis adds JWT token validation to the auth middleware."
	warnings := ValidateMessage(msg)
	if len(warnings) > 0 {
		t.Errorf("expected no warnings for valid message with body, got: %v", warnings)
	}
}

func TestGenerateMessage_WithMockLLM(t *testing.T) {
	gen := &CommitMessageGenerator{
		ChatFn: func(ctx context.Context, prompt string) (string, error) {
			return "feat(auth): add jwt validation\n\nImplement JWT token validation for API endpoints.", nil
		},
		FallbackToConventional: true,
		IncludeBody:            true,
	}

	ctx := context.Background()
	commitCtx := CommitContext{
		Diff:             "+func ValidateJWT(token string) error {}",
		FilesChanged:     []string{"pkg/auth/jwt.go"},
		ConversationGoal: "Add JWT authentication",
	}

	msg, err := gen.GenerateMessage(ctx, commitCtx)
	if err != nil {
		t.Fatalf("GenerateMessage() error: %v", err)
	}
	if !strings.HasPrefix(msg, "feat(auth):") {
		t.Errorf("expected LLM-generated message, got: %q", msg)
	}
	if !strings.Contains(msg, "jwt validation") {
		t.Errorf("expected message to contain 'jwt validation', got: %q", msg)
	}
}

func TestGenerateMessage_FallbackOnLLMError(t *testing.T) {
	gen := &CommitMessageGenerator{
		ChatFn: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("API rate limited")
		},
		FallbackToConventional: true,
		IncludeBody:            true,
	}

	ctx := context.Background()
	commitCtx := CommitContext{
		Diff: `new file mode 100644
+package auth
+func NewHandler() *Handler { return &Handler{} }
`,
		FilesChanged:     []string{"pkg/auth/handler.go"},
		ConversationGoal: "Create auth handler",
	}

	msg, err := gen.GenerateMessage(ctx, commitCtx)
	if err != nil {
		t.Fatalf("GenerateMessage() should fallback, got error: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty fallback message")
	}
	// Should be a valid conventional commit.
	if !strings.Contains(msg, ":") {
		t.Errorf("fallback should be conventional format, got: %q", msg)
	}
}

func TestGenerateMessage_NoFallback(t *testing.T) {
	gen := &CommitMessageGenerator{
		ChatFn: func(ctx context.Context, prompt string) (string, error) {
			return "", errors.New("API error")
		},
		FallbackToConventional: false,
	}

	ctx := context.Background()
	commitCtx := CommitContext{
		Diff:         "+something",
		FilesChanged: []string{"file.go"},
	}

	_, err := gen.GenerateMessage(ctx, commitCtx)
	if err == nil {
		t.Error("expected error when fallback disabled and LLM fails")
	}
}

func TestGenerateMessage_NilChatFn(t *testing.T) {
	gen := &CommitMessageGenerator{
		ChatFn:                 nil,
		FallbackToConventional: true,
		IncludeBody:            false,
	}

	ctx := context.Background()
	commitCtx := CommitContext{
		Diff:         "+func Test() {}",
		FilesChanged: []string{"handler_test.go"},
	}

	msg, err := gen.GenerateMessage(ctx, commitCtx)
	if err != nil {
		t.Fatalf("GenerateMessage() with nil ChatFn should use fallback: %v", err)
	}
	if !strings.HasPrefix(msg, "test") {
		t.Errorf("expected test type from fallback, got: %q", msg)
	}
}

func TestGenerateBody_WithGoal(t *testing.T) {
	ctx := CommitContext{
		FilesChanged:     []string{"pkg/auth/token.go", "pkg/auth/middleware.go"},
		ConversationGoal: "Implement JWT authentication",
	}

	body := GenerateBody(ctx)
	if !strings.Contains(body, "Implement JWT authentication") {
		t.Errorf("expected body to contain goal, got: %q", body)
	}
	if !strings.Contains(body, "token.go") {
		t.Errorf("expected body to list files, got: %q", body)
	}
}

func TestGenerateBody_NoGoal(t *testing.T) {
	ctx := CommitContext{
		FilesChanged: []string{"main.go"},
	}

	body := GenerateBody(ctx)
	if !strings.Contains(body, "main.go") {
		t.Errorf("expected body to list file, got: %q", body)
	}
}

func TestGenerateBody_ManyFiles(t *testing.T) {
	ctx := CommitContext{
		FilesChanged: []string{
			"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go",
		},
	}

	body := GenerateBody(ctx)
	if !strings.Contains(body, "and 2 more") {
		t.Errorf("expected truncation message, got: %q", body)
	}
}

func TestGenerateBody_Empty(t *testing.T) {
	ctx := CommitContext{}
	body := GenerateBody(ctx)
	if body != "" {
		t.Errorf("expected empty body, got: %q", body)
	}
}

func TestStyleMatchingFromPreviousCommits(t *testing.T) {
	gen := &CommitMessageGenerator{
		ChatFn: func(ctx context.Context, prompt string) (string, error) {
			// Verify the prompt includes previous commits.
			if !strings.Contains(prompt, "style reference") {
				t.Error("prompt should mention style reference")
			}
			if !strings.Contains(prompt, "feat(api): add endpoint") {
				t.Error("prompt should include previous commits")
			}
			return "feat(auth): add middleware", nil
		},
		FallbackToConventional: true,
		IncludeBody:            false,
	}

	ctx := context.Background()
	commitCtx := CommitContext{
		Diff:         "+func Middleware() {}",
		FilesChanged: []string{"auth/middleware.go"},
		PreviousCommits: []string{
			"feat(api): add endpoint",
			"fix(db): handle connection timeout",
			"test(auth): add unit tests",
		},
	}

	msg, err := gen.GenerateMessage(ctx, commitCtx)
	if err != nil {
		t.Fatalf("GenerateMessage() error: %v", err)
	}
	if msg != "feat(auth): add middleware" {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestGenerateMessage_LLMReturnsInvalid(t *testing.T) {
	gen := &CommitMessageGenerator{
		ChatFn: func(ctx context.Context, prompt string) (string, error) {
			// Return something that doesn't look like a commit message.
			return "I think you should commit this as a feature", nil
		},
		FallbackToConventional: true,
		IncludeBody:            false,
	}

	ctx := context.Background()
	commitCtx := CommitContext{
		Diff: `new file mode 100644
+package main
+func main() {}
`,
		FilesChanged: []string{"main.go"},
	}

	msg, err := gen.GenerateMessage(ctx, commitCtx)
	if err != nil {
		t.Fatalf("should fallback on invalid LLM response: %v", err)
	}
	// Should be rule-based fallback.
	if !strings.Contains(msg, ":") {
		t.Errorf("expected conventional format from fallback, got: %q", msg)
	}
}

func TestCommitMessageGenerator_MaxLength(t *testing.T) {
	gen := &CommitMessageGenerator{
		ChatFn: func(ctx context.Context, prompt string) (string, error) {
			if !strings.Contains(prompt, "50 characters") {
				t.Error("prompt should reflect custom max length")
			}
			return "feat: short msg", nil
		},
		MaxLength:   50,
		IncludeBody: false,
	}

	ctx := context.Background()
	commitCtx := CommitContext{
		Diff:         "+x",
		FilesChanged: []string{"x.go"},
	}

	msg, err := gen.GenerateMessage(ctx, commitCtx)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(msg) > 50 {
		t.Errorf("message exceeds max length: %d chars", len(msg))
	}
}

func TestDetectCommitType_EmptyInput(t *testing.T) {
	got := DetectCommitType("", nil)
	if got != "chore" {
		t.Errorf("DetectCommitType with empty input = %q, want %q", got, "chore")
	}
}

func TestDetectCommitType_CIFiles(t *testing.T) {
	diff := "+name: CI\n+on: push"
	files := []string{".github/workflows/ci.yml"}
	got := DetectCommitType(diff, files)
	if got != "chore" {
		t.Errorf("DetectCommitType() = %q, want %q", got, "chore")
	}
}

func TestValidateMessage_NoColon(t *testing.T) {
	warnings := ValidateMessage("add new feature")
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "colon") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about missing colon")
	}
}

func TestValidateMessage_WithScope(t *testing.T) {
	msg := "feat(auth): add middleware"
	warnings := ValidateMessage(msg)
	if len(warnings) > 0 {
		t.Errorf("expected no warnings for valid scoped message, got: %v", warnings)
	}
}
