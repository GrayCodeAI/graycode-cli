package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/ui/icons"
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

func TestLoadFromProject_JSONConfig(t *testing.T) {
	dir := t.TempDir()
	config := `{
  "extends": ["@commitlint/config-conventional"],
  "rules": {
    "type-enum": [2, "always", ["feat", "fix", "chore"]],
    "subject-max-length": [2, "always", 50],
    "header-max-length": [1, "always", 80]
  }
}`
	err := os.WriteFile(filepath.Join(dir, ".commitlintrc.json"), []byte(config), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	linter := NewCommitLinter()
	if err := linter.LoadFromProject(dir); err != nil {
		t.Fatalf("LoadFromProject() error: %v", err)
	}

	// Verify type-enum was updated.
	for _, rule := range linter.Rules {
		if rule.Name == "type-enum" {
			allowed, ok := rule.Value.([]string)
			if !ok {
				t.Fatalf("type-enum value is not []string: %T", rule.Value)
			}
			if len(allowed) != 3 {
				t.Errorf("expected 3 allowed types, got %d: %v", len(allowed), allowed)
			}
			break
		}
	}

	// Verify subject-max-length was updated.
	for _, rule := range linter.Rules {
		if rule.Name == "subject-max-length" {
			v, ok := rule.Value.(int)
			if !ok {
				t.Fatalf("subject-max-length value is not int: %T", rule.Value)
			}
			if v != 50 {
				t.Errorf("expected subject-max-length = 50, got %d", v)
			}
			break
		}
	}

	// Verify header-max-length is now a warning.
	for _, rule := range linter.Rules {
		if rule.Name == "header-max-length" {
			if rule.Level != "warning" {
				t.Errorf("expected header-max-length level = warning, got %q", rule.Level)
			}
			break
		}
	}
}

func TestLoadFromProject_JSConfig(t *testing.T) {
	dir := t.TempDir()
	config := `module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'type-enum': [2, 'always', ['feat', 'fix', 'docs', 'refactor']],
    'subject-max-length': [2, 'always', 60],
  },
};
`
	err := os.WriteFile(filepath.Join(dir, "commitlint.config.js"), []byte(config), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	linter := NewCommitLinter()
	if err := linter.LoadFromProject(dir); err != nil {
		t.Fatalf("LoadFromProject() error: %v", err)
	}

	// Verify type-enum was updated from JS config.
	for _, rule := range linter.Rules {
		if rule.Name == "type-enum" {
			allowed, ok := rule.Value.([]string)
			if !ok {
				t.Fatalf("type-enum value is not []string: %T", rule.Value)
			}
			if len(allowed) != 4 {
				t.Errorf("expected 4 allowed types, got %d: %v", len(allowed), allowed)
			}
			break
		}
	}
}

func TestLoadFromProject_NoConfig(t *testing.T) {
	dir := t.TempDir()
	linter := NewCommitLinter()
	err := linter.LoadFromProject(dir)
	if err == nil {
		t.Error("expected error when no config found")
	}
}

func TestLint_DisabledRule(t *testing.T) {
	linter := NewCommitLinter()
	// Disable type-enum.
	for i, rule := range linter.Rules {
		if rule.Name == "type-enum" {
			linter.Rules[i].Level = "disabled"
			break
		}
	}

	result := linter.Lint("yolo: anything goes")
	// Should not have type-enum error since it's disabled.
	for _, e := range result.Errors {
		if strings.Contains(e, "type-enum") {
			t.Errorf("disabled rule should not produce errors, got: %s", e)
		}
	}
}

func TestLint_AllValidTypes(t *testing.T) {
	linter := NewCommitLinter()
	types := []string{"feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"}

	for _, typ := range types {
		msg := typ + ": some description"
		result := linter.Lint(msg)
		for _, e := range result.Errors {
			if strings.Contains(e, "type-enum") {
				t.Errorf("type %q should be valid, got error: %s", typ, e)
			}
		}
	}
}

func TestParseCommitMessage_Empty(t *testing.T) {
	parsed := ParseCommitMessage("")
	if parsed.Type != "" || parsed.Scope != "" || parsed.Subject != "" {
		t.Errorf("expected empty ParsedCommit for empty input, got: %+v", parsed)
	}
}

func TestParseCommitMessage_NoColon(t *testing.T) {
	parsed := ParseCommitMessage("just a plain message")
	if parsed.Subject != "just a plain message" {
		t.Errorf("expected subject = %q, got %q", "just a plain message", parsed.Subject)
	}
}

func TestLint_BreakingChange(t *testing.T) {
	linter := NewCommitLinter()
	result := linter.Lint("feat!: breaking API change")

	if !result.Valid {
		t.Errorf("breaking change commit should be valid, errors: %v", result.Errors)
	}

	parsed := ParseCommitMessage("feat!: breaking API change")
	if !parsed.Breaking {
		t.Error("expected Breaking = true for '!' indicator")
	}
}

func TestCommitLinter_ConcurrentAccess(t *testing.T) {
	linter := NewCommitLinter()
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			result := linter.Lint("feat: concurrent test")
			if !result.Valid {
				t.Errorf("concurrent lint failed: %v", result.Errors)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestIsFooter(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Refs: #123", true},
		{"BREAKING CHANGE: removed old API", true},
		{"Reviewed-by: Alice", true},
		{"just some body text", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isFooter(tt.input)
			if got != tt.want {
				t.Errorf("isFooter(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestInferTypeFromContent(t *testing.T) {
	tests := []struct {
		subject string
		body    string
		want    string
	}{
		{"fix the login bug", "", "fix"},
		{"resolve null pointer error", "", "fix"},
		{"add new user authentication", "", "feat"},
		{"implement OAuth2 flow", "", "feat"},
		{"update readme", "", "docs"},
		{"add unit tests", "", "test"},
		{"restructure the codebase", "", "refactor"},
		{"format code with gofmt", "", "style"},
		{"optimize query performance", "", "perf"},
		{"bump dependency version", "", "chore"},
		{"", "", "chore"},
	}
	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			got := inferTypeFromContent(tt.subject, tt.body)
			if got != tt.want {
				t.Errorf("inferTypeFromContent(%q, %q) = %q, want %q", tt.subject, tt.body, got, tt.want)
			}
		})
	}
}

func TestSplitRuleValue(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"2, always, [feat,fix]", 3},
		{"2, always", 2},
		{"2, always, 50", 3},
		{"single", 1},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitRuleValue(tt.input)
			if len(got) != tt.want {
				t.Errorf("splitRuleValue(%q) returned %d parts, want %d: %v", tt.input, len(got), tt.want, got)
			}
		})
	}
}

func TestParseJSValue(t *testing.T) {
	tests := []struct {
		input string
		want  interface{}
	}{
		{"['feat','fix','docs']", []string{"feat", "fix", "docs"}},
		{"42", 42},
		{"'lowercase'", "lowercase"},
		{`"lowercase"`, "lowercase"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseJSValue(tt.input)
			switch want := tt.want.(type) {
			case []string:
				gotSlice, ok := got.([]string)
				if !ok {
					t.Errorf("parseJSValue(%q) returned %T, want []string", tt.input, got)
					return
				}
				if len(gotSlice) != len(want) {
					t.Errorf("parseJSValue(%q) returned %d items, want %d", tt.input, len(gotSlice), len(want))
				}
			case int:
				gotInt, ok := got.(int)
				if !ok {
					t.Errorf("parseJSValue(%q) returned %T, want int", tt.input, got)
					return
				}
				if gotInt != want {
					t.Errorf("parseJSValue(%q) = %d, want %d", tt.input, gotInt, want)
				}
			case string:
				gotStr, ok := got.(string)
				if !ok {
					t.Errorf("parseJSValue(%q) returned %T, want string", tt.input, got)
					return
				}
				if gotStr != want {
					t.Errorf("parseJSValue(%q) = %q, want %q", tt.input, gotStr, want)
				}
			}
		})
	}
}

func TestIsLowerCase(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"feat", true},
		{"Feat", false},
		{"FEAT", false},
		{"", true},
		{"fix123", true},
		{"FIX", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isLowerCase(tt.input)
			if got != tt.want {
				t.Errorf("isLowerCase(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLoadFromProject_YAMLConfig(t *testing.T) {
	dir := t.TempDir()
	config := `extends:
  - '@commitlint/config-conventional'
rules:
  type-enum: [2, always, 'feat|fix|chore']
  subject-max-length: [2, always, 50]
`
	err := os.WriteFile(filepath.Join(dir, ".commitlintrc.yml"), []byte(config), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	linter := NewCommitLinter()
	if err := linter.LoadFromProject(dir); err != nil {
		t.Fatalf("LoadFromProject() error: %v", err)
	}

	// Verify subject-max-length was updated
	for _, rule := range linter.Rules {
		if rule.Name == "subject-max-length" {
			v, ok := rule.Value.(int)
			if !ok {
				t.Fatalf("subject-max-length value is not int: %T", rule.Value)
			}
			if v != 50 {
				t.Errorf("expected subject-max-length = 50, got %d", v)
			}
			break
		}
	}
}

func TestLoadFromProject_YAMLAlternate(t *testing.T) {
	dir := t.TempDir()
	config := `rules:
  header-max-length: [1, always, 80]
`
	err := os.WriteFile(filepath.Join(dir, ".commitlintrc.yaml"), []byte(config), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	linter := NewCommitLinter()
	if err := linter.LoadFromProject(dir); err != nil {
		t.Fatalf("LoadFromProject() error: %v", err)
	}

	// Verify header-max-length was updated
	for _, rule := range linter.Rules {
		if rule.Name == "header-max-length" {
			if rule.Level != "warning" {
				t.Errorf("expected header-max-length level = warning, got %q", rule.Level)
			}
			break
		}
	}
}

func TestLint_UnknownRule(t *testing.T) {
	linter := NewCommitLinter()
	linter.Rules = append(linter.Rules, CommitRule{
		Name:       "unknown-rule",
		Level:      "error",
		Applicable: "always",
	})
	// Unknown rules should be silently ignored
	result := linter.Lint("feat: add login")
	if !result.Valid {
		t.Errorf("unknown rule should not cause validation failure: %v", result.Errors)
	}
}

func TestCheckSubjectMaxLength_Float64Value(t *testing.T) {
	linter := NewCommitLinter()
	// Override with float64 value (as JSON parsing might produce)
	linter.Rules = []CommitRule{
		{Name: "subject-max-length", Level: "error", Applicable: "always", Value: float64(50)},
	}
	msg := "feat: " + strings.Repeat("a", 60)
	result := linter.Lint(msg)
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "subject-max-length") {
			found = true
		}
	}
	if !found {
		t.Error("expected subject-max-length error with float64 value")
	}
}

func TestCheckBodyMaxLineLength_Float64Value(t *testing.T) {
	linter := NewCommitLinter()
	linter.Rules = []CommitRule{
		{Name: "body-max-line-length", Level: "warning", Applicable: "always", Value: float64(80)},
	}
	longLine := strings.Repeat("x", 100)
	msg := "feat: add\n\n" + longLine
	result := linter.Lint(msg)
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "body-max-line-length") {
			found = true
		}
	}
	if !found {
		t.Error("expected body-max-line-length warning with float64 value")
	}
}

func TestCheckHeaderMaxLength_Float64Value(t *testing.T) {
	linter := NewCommitLinter()
	linter.Rules = []CommitRule{
		{Name: "header-max-length", Level: "error", Applicable: "always", Value: float64(50)},
	}
	msg := "feat: " + strings.Repeat("b", 60)
	result := linter.Lint(msg)
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "header-max-length") {
			found = true
		}
	}
	if !found {
		t.Error("expected header-max-length error with float64 value")
	}
}

func TestCheckFooterMaxLineLength_Float64Value(t *testing.T) {
	linter := NewCommitLinter()
	linter.Rules = []CommitRule{
		{Name: "footer-max-line-length", Level: "warning", Applicable: "always", Value: float64(80)},
	}
	longFooter := strings.Repeat("z", 100)
	msg := "feat: add\n\nBody.\n\nRefs: #123\n" + longFooter
	result := linter.Lint(msg)
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "footer-max-line-length") {
			found = true
		}
	}
	if !found {
		t.Error("expected footer-max-line-length warning with float64 value")
	}
}

func TestCheckTypeEnum_NeverApplicable(t *testing.T) {
	linter := NewCommitLinter()
	for i, rule := range linter.Rules {
		if rule.Name == "type-enum" {
			linter.Rules[i].Applicable = "never"
			break
		}
	}
	result := linter.Lint("feat: add login")
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "type-enum") {
			found = true
		}
	}
	if !found {
		t.Error("expected type-enum error with 'never' applicable")
	}
}

func TestCheckTypeCase_EmptyCaseType(t *testing.T) {
	linter := NewCommitLinter()
	for i, rule := range linter.Rules {
		if rule.Name == "type-case" {
			linter.Rules[i].Value = ""
			break
		}
	}
	// Should default to lowercase check
	result := linter.Lint("Feat: add something")
	if result.Valid {
		t.Error("expected invalid result for uppercase type with empty case type")
	}
}

func TestCheckScopeCase_Uppercase(t *testing.T) {
	linter := NewCommitLinter()
	for i, rule := range linter.Rules {
		if rule.Name == "scope-case" {
			linter.Rules[i].Value = "uppercase"
			break
		}
	}
	result := linter.Lint("feat(AUTH): add login")
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "scope-case") {
			found = true
		}
	}
	// With uppercase rule, lowercase scope should fail
	if found {
		t.Error("unexpected scope-case error for uppercase scope with uppercase rule")
	}
}

func TestCheckSubjectMaxLength_EmptyCaseType(t *testing.T) {
	linter := NewCommitLinter()
	for i, rule := range linter.Rules {
		if rule.Name == "subject-max-length" {
			linter.Rules[i].Value = nil
			break
		}
	}
	// Should use default 72
	result := linter.Lint("feat: " + strings.Repeat("a", 80))
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "subject-max-length") {
			found = true
		}
	}
	if !found {
		t.Error("expected subject-max-length error with nil value (default 72)")
	}
}

func TestParseCommitMessage_JustType(t *testing.T) {
	parsed := ParseCommitMessage("feat:")
	if parsed.Type != "feat" {
		t.Errorf("Type = %q, want %q", parsed.Type, "feat")
	}
	if parsed.Subject != "" {
		t.Errorf("Subject = %q, want empty", parsed.Subject)
	}
}

func TestParseCommitMessage_BreakingInBody(t *testing.T) {
	msg := "feat: add auth\n\nImplement OAuth2.\n\nBREAKING CHANGE: removed legacy"
	parsed := ParseCommitMessage(msg)
	if !parsed.Breaking {
		t.Error("expected Breaking = true for BREAKING CHANGE in body")
	}
}

func TestParseCommitMessage_BreakingDashChange(t *testing.T) {
	msg := "feat: add auth\n\nBREAKING-CHANGE: removed legacy"
	parsed := ParseCommitMessage(msg)
	if !parsed.Breaking {
		t.Error("expected Breaking = true for BREAKING-CHANGE")
	}
}

func TestFixMessage_WithScope(t *testing.T) {
	linter := NewCommitLinter()
	fixed := linter.FixMessage("Feat(Auth): add login")
	if !strings.HasPrefix(fixed, "feat(auth):") {
		t.Errorf("expected 'feat(auth):' prefix, got: %q", fixed)
	}
}

func TestFixMessage_LongHeader(t *testing.T) {
	linter := NewCommitLinter()
	long := "feat: " + strings.Repeat("a", 120)
	fixed := linter.FixMessage(long)
	if len(fixed) > 100 {
		t.Errorf("expected header truncated to 100 chars, got %d: %q", len(fixed), fixed)
	}
}

func TestUpdateRule_NewRule(t *testing.T) {
	linter := NewCommitLinter()
	initialCount := len(linter.Rules)
	linter.updateRule(CommitRule{
		Name:       "custom-rule",
		Level:      "error",
		Applicable: "always",
		Value:      "test",
	})
	if len(linter.Rules) != initialCount+1 {
		t.Errorf("expected %d rules, got %d", initialCount+1, len(linter.Rules))
	}
}

func TestUpdateRule_OverrideExisting(t *testing.T) {
	linter := NewCommitLinter()
	linter.updateRule(CommitRule{
		Name:       "type-enum",
		Level:      "warning",
		Applicable: "never",
		Value:      []string{"a", "b"},
	})
	for _, rule := range linter.Rules {
		if rule.Name == "type-enum" {
			if rule.Level != "warning" {
				t.Errorf("expected level 'warning', got %q", rule.Level)
			}
			if rule.Applicable != "never" {
				t.Errorf("expected applicable 'never', got %q", rule.Applicable)
			}
			return
		}
	}
	t.Error("type-enum rule not found")
}
