package tool

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func TestNewCommitLinter_DefaultRules(t *testing.T) {
	linter := NewCommitLinter()
	if len(linter.Rules) == 0 {
		t.Fatal("expected default rules, got none")
	}

	ruleNames := make(map[string]bool)
	for _, r := range linter.Rules {
		ruleNames[r.Name] = true
	}

	expected := []string{
		"type-enum", "type-case", "scope-case",
		"subject-max-length", "subject-empty",
		"body-max-line-length", "header-max-length",
		"footer-max-line-length",
	}
	for _, name := range expected {
		if !ruleNames[name] {
			t.Errorf("missing default rule %q", name)
		}
	}
}

func TestParseCommitMessage_BasicConventional(t *testing.T) {
	tests := []struct {
		input        string
		wantType     string
		wantScope    string
		wantSubj     string
		wantBreaking bool
	}{
		{
			input:    "feat: add new feature",
			wantType: "feat",
			wantSubj: "add new feature",
		},
		{
			input:     "fix(auth): resolve token expiry",
			wantType:  "fix",
			wantScope: "auth",
			wantSubj:  "resolve token expiry",
		},
		{
			input:        "refactor!: rewrite entire module",
			wantType:     "refactor",
			wantSubj:     "rewrite entire module",
			wantBreaking: true,
		},
		{
			input:     "docs(readme): update installation instructions",
			wantType:  "docs",
			wantScope: "readme",
			wantSubj:  "update installation instructions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			parsed := ParseCommitMessage(tt.input)
			if parsed.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", parsed.Type, tt.wantType)
			}
			if parsed.Scope != tt.wantScope {
				t.Errorf("Scope = %q, want %q", parsed.Scope, tt.wantScope)
			}
			if parsed.Subject != tt.wantSubj {
				t.Errorf("Subject = %q, want %q", parsed.Subject, tt.wantSubj)
			}
			if parsed.Breaking != tt.wantBreaking {
				t.Errorf("Breaking = %v, want %v", parsed.Breaking, tt.wantBreaking)
			}
		})
	}
}

func TestParseCommitMessage_WithBody(t *testing.T) {
	msg := "feat(parser): add support for arrays\n\nThis adds support for array types\nin the parser module."
	parsed := ParseCommitMessage(msg)

	if parsed.Type != "feat" {
		t.Errorf("Type = %q, want %q", parsed.Type, "feat")
	}
	if parsed.Scope != "parser" {
		t.Errorf("Scope = %q, want %q", parsed.Scope, "parser")
	}
	if parsed.Body == "" {
		t.Error("expected non-empty body")
	}
	if !strings.Contains(parsed.Body, "array types") {
		t.Errorf("Body = %q, expected to contain 'array types'", parsed.Body)
	}
}

func TestParseCommitMessage_WithFooter(t *testing.T) {
	msg := "feat: add auth module\n\nImplement OAuth2 flow.\n\nBREAKING CHANGE: removed legacy auth\nRefs: #123"
	parsed := ParseCommitMessage(msg)

	if parsed.Type != "feat" {
		t.Errorf("Type = %q, want %q", parsed.Type, "feat")
	}
	if !parsed.Breaking {
		t.Error("expected Breaking = true")
	}
	if parsed.Footer == "" {
		t.Error("expected non-empty footer")
	}
	if !strings.Contains(parsed.Footer, "BREAKING CHANGE") {
		t.Errorf("Footer = %q, expected to contain 'BREAKING CHANGE'", parsed.Footer)
	}
}

func TestLint_ValidMessage(t *testing.T) {
	linter := NewCommitLinter()
	result := linter.Lint("feat(auth): add token refresh logic")

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
}

func TestLint_InvalidType(t *testing.T) {
	linter := NewCommitLinter()
	result := linter.Lint("yolo: ship it")

	if result.Valid {
		t.Error("expected invalid result for bad type")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "type-enum") && strings.Contains(e, "yolo") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected type-enum error mentioning 'yolo', got: %v", result.Errors)
	}
}

func TestLint_TypeCaseViolation(t *testing.T) {
	linter := NewCommitLinter()
	result := linter.Lint("Feat: add something")

	if result.Valid {
		t.Error("expected invalid result for uppercase type")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "type-case") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected type-case error, got: %v", result.Errors)
	}
}

func TestLint_ScopeCaseViolation(t *testing.T) {
	linter := NewCommitLinter()
	result := linter.Lint("feat(Auth): add login")

	if result.Valid {
		t.Error("expected invalid result for uppercase scope")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "scope-case") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected scope-case error, got: %v", result.Errors)
	}
}

func TestLint_SubjectTooLong(t *testing.T) {
	linter := NewCommitLinter()
	longSubject := "feat: " + strings.Repeat("a", 80)
	result := linter.Lint(longSubject)

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "subject-max-length") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected subject-max-length error, got errors: %v, warnings: %v", result.Errors, result.Warnings)
	}
}

func TestLint_EmptySubject(t *testing.T) {
	linter := NewCommitLinter()
	result := linter.Lint("feat: ")

	if result.Valid {
		t.Error("expected invalid result for empty subject")
	}

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "subject-empty") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected subject-empty error, got: %v", result.Errors)
	}
}

func TestLint_EmptyMessage(t *testing.T) {
	linter := NewCommitLinter()
	result := linter.Lint("")

	if result.Valid {
		t.Error("expected invalid for empty message")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error for empty message")
	}
}

func TestLint_BodyMaxLineLength(t *testing.T) {
	linter := NewCommitLinter()
	longLine := strings.Repeat("x", 120)
	msg := "feat: add thing\n\n" + longLine
	result := linter.Lint(msg)

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "body-max-line-length") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected body-max-line-length warning, got warnings: %v", result.Warnings)
	}
}

func TestLint_HeaderMaxLength(t *testing.T) {
	linter := NewCommitLinter()
	longHeader := "feat: " + strings.Repeat("b", 100)
	result := linter.Lint(longHeader)

	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "header-max-length") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected header-max-length error, got: %v", result.Errors)
	}
}

func TestLint_FooterMaxLineLength(t *testing.T) {
	linter := NewCommitLinter()
	longFooter := strings.Repeat("z", 120)
	msg := "feat: add module\n\nSome body.\n\nRefs: #123\n" + longFooter
	result := linter.Lint(msg)

	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "footer-max-line-length") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected footer-max-line-length warning, got warnings: %v", result.Warnings)
	}
}

func TestFixMessage_UppercaseType(t *testing.T) {
	linter := NewCommitLinter()
	fixed := linter.FixMessage("Feat: add login")

	if !strings.HasPrefix(fixed, "feat:") {
		t.Errorf("expected lowercase type, got: %q", fixed)
	}
}

func TestFixMessage_LongSubject(t *testing.T) {
	linter := NewCommitLinter()
	long := "feat: " + strings.Repeat("a", 100)
	fixed := linter.FixMessage(long)

	parsed := ParseCommitMessage(fixed)
	if len(parsed.Subject) > 72 {
		t.Errorf("expected subject truncated to 72 chars, got %d", len(parsed.Subject))
	}
	if !strings.HasSuffix(parsed.Subject, "...") {
		t.Error("expected truncated subject to end with '...'")
	}
}

func TestFixMessage_MissingType(t *testing.T) {
	linter := NewCommitLinter()
	fixed := linter.FixMessage("add new authentication module")

	parsed := ParseCommitMessage(fixed)
	if parsed.Type == "" {
		t.Error("expected type to be inferred")
	}
	if parsed.Type != "feat" {
		t.Errorf("expected inferred type 'feat', got %q", parsed.Type)
	}
}

func TestFixMessage_Empty(t *testing.T) {
	linter := NewCommitLinter()
	fixed := linter.FixMessage("")
	if fixed != "" {
		t.Errorf("expected empty string for empty input, got %q", fixed)
	}
}

func TestFormatLintResult_Valid(t *testing.T) {
	result := &LintResult{Valid: true}
	output := FormatLintResult(result)

	if !strings.Contains(output, icons.CheckBold()) {
		t.Errorf("expected checkmark for valid result, got: %q", output)
	}
}

func TestFormatLintResult_WithErrors(t *testing.T) {
	result := &LintResult{
		Valid:  false,
		Errors: []string{"type-enum: \"yolo\" is not in [feat, fix, ...]"},
	}
	output := FormatLintResult(result)

	if !strings.Contains(output, icons.Alert()) {
		t.Errorf("expected warning symbol, got: %q", output)
	}
	if !strings.Contains(output, icons.CloseThick()) {
		t.Errorf("expected error cross, got: %q", output)
	}
	if !strings.Contains(output, "yolo") {
		t.Errorf("expected error message content, got: %q", output)
	}
}

func TestFormatLintResult_WithWarnings(t *testing.T) {
	result := &LintResult{
		Valid:    true,
		Warnings: []string{"body-max-line-length: body line is 120 chars, max 100"},
	}
	output := FormatLintResult(result)

	if !strings.Contains(output, "body-max-line-length") {
		t.Errorf("expected warning content, got: %q", output)
	}
}
