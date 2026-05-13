package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseCommits_Basic(t *testing.T) {
	gitLog := `abc1234|Alice|feat(auth): add JWT token refresh
def5678|Bob|fix(handler): handle nil pointer in error path
ghi9012|Alice|refactor(config): simplify validation logic`

	commits := ParseCommits(gitLog)

	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}

	// First commit.
	if commits[0].Hash != "abc1234" {
		t.Errorf("commits[0].Hash = %q, want %q", commits[0].Hash, "abc1234")
	}
	if commits[0].Author != "Alice" {
		t.Errorf("commits[0].Author = %q, want %q", commits[0].Author, "Alice")
	}
	if commits[0].Type != "feat" {
		t.Errorf("commits[0].Type = %q, want %q", commits[0].Type, "feat")
	}
	if commits[0].Scope != "auth" {
		t.Errorf("commits[0].Scope = %q, want %q", commits[0].Scope, "auth")
	}
	if commits[0].Message != "feat(auth): add JWT token refresh" {
		t.Errorf("commits[0].Message = %q", commits[0].Message)
	}

	// Second commit.
	if commits[1].Type != "fix" {
		t.Errorf("commits[1].Type = %q, want %q", commits[1].Type, "fix")
	}
	if commits[1].Scope != "handler" {
		t.Errorf("commits[1].Scope = %q, want %q", commits[1].Scope, "handler")
	}

	// Third commit.
	if commits[2].Type != "refactor" {
		t.Errorf("commits[2].Type = %q, want %q", commits[2].Type, "refactor")
	}
}

func TestParseCommits_NoConventionalFormat(t *testing.T) {
	gitLog := `abc1234|Dev|update readme
def5678|Dev|fix something`

	commits := ParseCommits(gitLog)

	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	// "update readme" has no colon, so type should be empty.
	if commits[0].Type != "" {
		t.Errorf("commits[0].Type = %q, want empty", commits[0].Type)
	}

	// "fix something" has no colon either.
	if commits[1].Type != "" {
		t.Errorf("commits[1].Type = %q, want empty", commits[1].Type)
	}
}

func TestParseCommits_WithBreaking(t *testing.T) {
	gitLog := `abc1234|Dev|feat!: remove deprecated API`

	commits := ParseCommits(gitLog)
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0].Type != "feat" {
		t.Errorf("Type = %q, want %q", commits[0].Type, "feat")
	}
}

func TestParseCommits_EmptyInput(t *testing.T) {
	commits := ParseCommits("")
	if len(commits) != 0 {
		t.Errorf("expected 0 commits for empty input, got %d", len(commits))
	}
}

func TestParseCommits_MalformedLine(t *testing.T) {
	gitLog := `abc1234|incomplete`
	commits := ParseCommits(gitLog)
	if len(commits) != 0 {
		t.Errorf("expected 0 commits for malformed input, got %d", len(commits))
	}
}

func TestGenerateTitle_SingleCommit(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat(auth): add JWT validation", Type: "feat", Scope: "auth"},
	}

	title := GenerateTitle(commits)
	if title != "feat(auth): add JWT validation" {
		t.Errorf("GenerateTitle() = %q", title)
	}
}

func TestGenerateTitle_SingleCommitTruncation(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat(auth): this is a really long commit message that exceeds seventy two characters in length definitely", Type: "feat", Scope: "auth"},
	}

	title := GenerateTitle(commits)
	if len(title) > 72 {
		t.Errorf("title exceeds 72 chars: len=%d, title=%q", len(title), title)
	}
	if !strings.HasSuffix(title, "...") {
		t.Errorf("truncated title should end with ..., got %q", title)
	}
}

func TestGenerateTitle_SameType(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat(auth): add login", Type: "feat", Scope: "auth"},
		{Hash: "def", Message: "feat(auth): add logout", Type: "feat", Scope: "auth"},
	}

	title := GenerateTitle(commits)
	if !strings.HasPrefix(title, "feat(auth):") {
		t.Errorf("expected title to start with 'feat(auth):', got %q", title)
	}
}

func TestGenerateTitle_SameTypeNoScope(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "fix: correct typo", Type: "fix", Scope: ""},
		{Hash: "def", Message: "fix: handle edge case", Type: "fix", Scope: ""},
	}

	title := GenerateTitle(commits)
	if !strings.HasPrefix(title, "fix:") {
		t.Errorf("expected title to start with 'fix:', got %q", title)
	}
}

func TestGenerateTitle_MixedTypes(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add login", Type: "feat", Scope: ""},
		{Hash: "def", Message: "fix: handle error", Type: "fix", Scope: ""},
		{Hash: "ghi", Message: "refactor: clean up", Type: "refactor", Scope: ""},
	}

	title := GenerateTitle(commits)
	if !strings.HasPrefix(title, "Multiple changes:") {
		t.Errorf("expected 'Multiple changes:' prefix, got %q", title)
	}
	if !strings.Contains(title, "feat") {
		t.Errorf("expected title to contain 'feat', got %q", title)
	}
	if !strings.Contains(title, "fix") {
		t.Errorf("expected title to contain 'fix', got %q", title)
	}
}

func TestGenerateTitle_Empty(t *testing.T) {
	title := GenerateTitle(nil)
	if title != "Update" {
		t.Errorf("GenerateTitle(nil) = %q, want %q", title, "Update")
	}
}

func TestGenerateTitle_MaxLength(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add login", Type: "feat"},
		{Hash: "def", Message: "feat: add registration", Type: "feat"},
		{Hash: "ghi", Message: "feat: add password reset", Type: "feat"},
		{Hash: "jkl", Message: "feat: add email verification", Type: "feat"},
	}

	title := GenerateTitle(commits)
	if len(title) > 72 {
		t.Errorf("title exceeds 72 chars: len=%d", len(title))
	}
}

func TestGeneratePRBody_Sections(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat(auth): add JWT token refresh", Type: "feat", Scope: "auth"},
		{Hash: "def", Message: "fix(handler): handle nil pointer", Type: "fix", Scope: "handler"},
	}
	diffStat := " 5 files changed, 120 insertions(+), 30 deletions(-)"
	files := []string{"pkg/auth/token.go", "pkg/handler/api.go"}

	body := GeneratePRBody(commits, diffStat, files)

	if !strings.Contains(body, "## Summary") {
		t.Error("body missing ## Summary section")
	}
	if !strings.Contains(body, "## Changes") {
		t.Error("body missing ## Changes section")
	}
	if !strings.Contains(body, "## Files Changed") {
		t.Error("body missing ## Files Changed section")
	}
	if !strings.Contains(body, "## Test Plan") {
		t.Error("body missing ## Test Plan section")
	}
	if !strings.Contains(body, "## Breaking Changes") {
		t.Error("body missing ## Breaking Changes section")
	}
	if !strings.Contains(body, "None") {
		t.Error("expected 'None' for breaking changes")
	}
	if !strings.Contains(body, "feat(auth)") {
		t.Error("body should list feat(auth) commit")
	}
	if !strings.Contains(body, "fix(handler)") {
		t.Error("body should list fix(handler) commit")
	}
	if !strings.Contains(body, "pkg/auth/token.go") {
		t.Error("body should list changed files")
	}
}

func TestGeneratePRBody_EmptyDiffStat(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "chore: update deps", Type: "chore"},
	}

	body := GeneratePRBody(commits, "", nil)
	if !strings.Contains(body, "## Summary") {
		t.Error("body missing summary even with empty diff stat")
	}
}

func TestGeneratePRBody_BreakingChanges(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat!: remove deprecated API endpoint", Type: "feat"},
	}
	files := []string{"api/handler.go"}

	body := GeneratePRBody(commits, "", files)

	if strings.Contains(body, "## Breaking Changes\nNone") {
		t.Error("should detect breaking change from '!' in commit")
	}
	if !strings.Contains(body, "remove deprecated API endpoint") {
		t.Error("body should mention the breaking change")
	}
}

func TestSuggestLabels_Feature(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add login", Type: "feat"},
	}

	labels := SuggestLabels(commits)
	if !containsStr(labels, "feature") {
		t.Errorf("expected 'feature' label, got %v", labels)
	}
}

func TestSuggestLabels_Bugfix(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "fix: handle nil", Type: "fix"},
	}

	labels := SuggestLabels(commits)
	if !containsStr(labels, "bugfix") {
		t.Errorf("expected 'bugfix' label, got %v", labels)
	}
}

func TestSuggestLabels_Breaking(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat!: BREAKING CHANGE remove old API", Type: "feat"},
	}

	labels := SuggestLabels(commits)
	if !containsStr(labels, "breaking") {
		t.Errorf("expected 'breaking' label, got %v", labels)
	}
}

func TestSuggestLabels_Documentation(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "docs: update readme", Type: "docs"},
	}

	labels := SuggestLabels(commits)
	if !containsStr(labels, "documentation") {
		t.Errorf("expected 'documentation' label, got %v", labels)
	}
}

func TestSuggestLabels_Dependencies(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "chore: update deps", Type: "chore", Files: []string{"go.mod", "go.sum"}},
	}

	labels := SuggestLabels(commits)
	if !containsStr(labels, "dependencies") {
		t.Errorf("expected 'dependencies' label, got %v", labels)
	}
}

func TestSuggestLabels_Multiple(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add login", Type: "feat"},
		{Hash: "def", Message: "fix: handle error", Type: "fix"},
	}

	labels := SuggestLabels(commits)
	if !containsStr(labels, "feature") {
		t.Errorf("expected 'feature' label, got %v", labels)
	}
	if !containsStr(labels, "bugfix") {
		t.Errorf("expected 'bugfix' label, got %v", labels)
	}
}

func TestSuggestReviewers_Basic(t *testing.T) {
	files := []string{"auth/token.go", "auth/middleware.go", "handler/api.go"}
	blame := map[string]string{
		"auth/token.go":      "Alice",
		"auth/middleware.go": "Alice",
		"handler/api.go":     "Bob",
	}

	reviewers := SuggestReviewers(files, blame)
	if len(reviewers) == 0 {
		t.Fatal("expected at least one reviewer")
	}
	// Alice has more files so should be first.
	if reviewers[0] != "Alice" {
		t.Errorf("expected first reviewer to be Alice, got %q", reviewers[0])
	}
}

func TestSuggestReviewers_Empty(t *testing.T) {
	reviewers := SuggestReviewers(nil, nil)
	if reviewers != nil {
		t.Errorf("expected nil reviewers for empty input, got %v", reviewers)
	}
}

func TestSuggestReviewers_NoBlameData(t *testing.T) {
	files := []string{"auth/token.go"}
	blame := map[string]string{}

	reviewers := SuggestReviewers(files, blame)
	if reviewers != nil {
		t.Errorf("expected nil reviewers when no blame data, got %v", reviewers)
	}
}

func TestSuggestReviewers_MaxThree(t *testing.T) {
	files := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	blame := map[string]string{
		"a.go": "Alice",
		"b.go": "Bob",
		"c.go": "Charlie",
		"d.go": "Dave",
		"e.go": "Eve",
	}

	reviewers := SuggestReviewers(files, blame)
	if len(reviewers) > 3 {
		t.Errorf("expected at most 3 reviewers, got %d", len(reviewers))
	}
}

func TestGenerateTestPlan_GoFiles(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add auth", Type: "feat"},
	}
	files := []string{"pkg/auth/auth.go", "pkg/auth/auth_test.go"}

	plan := GenerateTestPlan(commits, files)
	if !strings.Contains(plan, "go test") {
		t.Errorf("expected 'go test' in test plan, got %q", plan)
	}
}

func TestGenerateTestPlan_JSFiles(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add component", Type: "feat"},
	}
	files := []string{"src/App.tsx", "src/App.test.tsx"}

	plan := GenerateTestPlan(commits, files)
	if !strings.Contains(plan, "npm test") {
		t.Errorf("expected 'npm test' in test plan, got %q", plan)
	}
}

func TestGenerateTestPlan_PythonFiles(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add model", Type: "feat"},
	}
	files := []string{"models/user.py"}

	plan := GenerateTestPlan(commits, files)
	if !strings.Contains(plan, "pytest") {
		t.Errorf("expected 'pytest' in test plan, got %q", plan)
	}
}

func TestGenerateTestPlan_APIChanges(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add endpoint", Type: "feat"},
	}
	files := []string{"pkg/api/handler.go"}

	plan := GenerateTestPlan(commits, files)
	if !strings.Contains(plan, "API endpoints") {
		t.Errorf("expected API test suggestion, got %q", plan)
	}
}

func TestGenerateTestPlan_DatabaseChanges(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add migration", Type: "feat"},
	}
	files := []string{"db/migrations/001_create_users.sql"}

	plan := GenerateTestPlan(commits, files)
	if !strings.Contains(plan, "migration") {
		t.Errorf("expected migration test suggestion, got %q", plan)
	}
}

func TestGenerateTestPlan_BreakingChange(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat!: remove old API", Type: "feat"},
	}
	files := []string{"api/v2/handler.go"}

	plan := GenerateTestPlan(commits, files)
	if !strings.Contains(plan, "backward compatibility") {
		t.Errorf("expected backward compatibility check, got %q", plan)
	}
}

func TestGenerateTestPlan_NoBreaking(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add helper", Type: "feat"},
	}
	files := []string{"utils/helper.go"}

	plan := GenerateTestPlan(commits, files)
	if !strings.Contains(plan, "No breaking changes") {
		t.Errorf("expected no breaking changes note, got %q", plan)
	}
}

func TestFormatForGitHub_Basic(t *testing.T) {
	pr := &PRDescription{
		Title:     "feat: add authentication",
		Body:      "## Summary\nAdd auth module.",
		Labels:    []string{"feature"},
		Reviewers: []string{"alice", "bob"},
	}

	output := FormatForGitHub(pr)
	if !strings.Contains(output, "gh pr create") {
		t.Errorf("expected 'gh pr create' in output, got %q", output)
	}
	if !strings.Contains(output, "--title") {
		t.Errorf("expected --title flag, got %q", output)
	}
	if !strings.Contains(output, "--body") {
		t.Errorf("expected --body flag, got %q", output)
	}
	if !strings.Contains(output, "--label") {
		t.Errorf("expected --label flag, got %q", output)
	}
	if !strings.Contains(output, "--reviewer") {
		t.Errorf("expected --reviewer flag, got %q", output)
	}
}

func TestFormatForGitHub_NoLabelsOrReviewers(t *testing.T) {
	pr := &PRDescription{
		Title: "fix: correct typo",
		Body:  "## Summary\nFix typo.",
	}

	output := FormatForGitHub(pr)
	if strings.Contains(output, "--label") {
		t.Errorf("should not have --label without labels, got %q", output)
	}
	if strings.Contains(output, "--reviewer") {
		t.Errorf("should not have --reviewer without reviewers, got %q", output)
	}
}

func TestDetectBreaking_ExclamationMark(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat!: remove old endpoint"},
	}
	if !detectBreaking(commits) {
		t.Error("expected breaking change detection from '!' prefix")
	}
}

func TestDetectBreaking_BreakingChangeText(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: BREAKING CHANGE: new API format"},
	}
	if !detectBreaking(commits) {
		t.Error("expected breaking change detection from 'BREAKING CHANGE' text")
	}
}

func TestDetectBreaking_NoBreaking(t *testing.T) {
	commits := []CommitSummary{
		{Hash: "abc", Message: "feat: add new endpoint"},
	}
	if detectBreaking(commits) {
		t.Error("should not detect breaking change in normal commit")
	}
}

func TestDetectPRType_MostCommon(t *testing.T) {
	commits := []CommitSummary{
		{Type: "feat"},
		{Type: "feat"},
		{Type: "fix"},
	}
	got := detectPRType(commits)
	if got != "feat" {
		t.Errorf("detectPRType() = %q, want %q", got, "feat")
	}
}

func TestDetectPRType_Empty(t *testing.T) {
	got := detectPRType(nil)
	if got != "chore" {
		t.Errorf("detectPRType(nil) = %q, want %q", got, "chore")
	}
}

func TestParseConventionalCommit_WithScope(t *testing.T) {
	commitType, scope := parseConventionalCommit("feat(auth): add login")
	if commitType != "feat" {
		t.Errorf("type = %q, want %q", commitType, "feat")
	}
	if scope != "auth" {
		t.Errorf("scope = %q, want %q", scope, "auth")
	}
}

func TestParseConventionalCommit_NoScope(t *testing.T) {
	commitType, scope := parseConventionalCommit("fix: handle error")
	if commitType != "fix" {
		t.Errorf("type = %q, want %q", commitType, "fix")
	}
	if scope != "" {
		t.Errorf("scope = %q, want empty", scope)
	}
}

func TestParseConventionalCommit_Breaking(t *testing.T) {
	commitType, scope := parseConventionalCommit("feat(api)!: remove endpoint")
	if commitType != "feat" {
		t.Errorf("type = %q, want %q", commitType, "feat")
	}
	if scope != "api" {
		t.Errorf("scope = %q, want %q", scope, "api")
	}
}

func TestParseConventionalCommit_Invalid(t *testing.T) {
	commitType, scope := parseConventionalCommit("update readme")
	if commitType != "" {
		t.Errorf("type = %q, want empty for invalid format", commitType)
	}
	if scope != "" {
		t.Errorf("scope = %q, want empty for invalid format", scope)
	}
}

func TestParseConventionalCommit_InvalidType(t *testing.T) {
	commitType, _ := parseConventionalCommit("invalid: do something")
	if commitType != "" {
		t.Errorf("type = %q, want empty for invalid type", commitType)
	}
}

func TestStripConventionalPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feat(auth): add login", "add login"},
		{"fix: handle error", "handle error"},
		{"refactor(config): simplify logic", "simplify logic"},
		{"update readme", "update readme"},
		{"feat!: breaking change", "breaking change"},
	}

	for _, tt := range tests {
		got := stripConventionalPrefix(tt.input)
		if got != tt.want {
			t.Errorf("stripConventionalPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDescribeFile(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"pkg/auth/auth_test.go", "Tests"},
		{"go.mod", "Dependencies"},
		{"README.md", "Documentation"},
		{"config/settings.go", "Configuration"},
		{"api/handler.go", "API layer"},
		{"models/user.go", "Data model"},
		{"pkg/util/helper.go", ""},
	}

	for _, tt := range tests {
		got := describeFile(tt.path)
		if got != tt.want {
			t.Errorf("describeFile(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestSplitNonEmpty(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a\nb\nc", 3},
		{"a\n\nb", 2},
		{"", 0},
		{"\n\n\n", 0},
		{"  a  \n  b  ", 2},
	}

	for _, tt := range tests {
		got := splitNonEmpty(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitNonEmpty(%q) got %d items, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestNewPRGenerator(t *testing.T) {
	gen := NewPRGenerator("/tmp/project")
	if gen == nil {
		t.Fatal("NewPRGenerator returned nil")
	}
	if gen.ProjectDir != "/tmp/project" {
		t.Errorf("ProjectDir = %q, want %q", gen.ProjectDir, "/tmp/project")
	}
}

func TestPRGeneratorTool_Interface(t *testing.T) {
	var tool Tool = &PRGeneratorTool{ProjectDir: "/tmp"}
	if tool.Name() != "pr_generate" {
		t.Errorf("Name() = %q, want %q", tool.Name(), "pr_generate")
	}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
	params := tool.Parameters()
	if params == nil {
		t.Error("Parameters() should not be nil")
	}
	props, ok := params["properties"]
	if !ok {
		t.Error("Parameters() should have 'properties' key")
	}
	propsMap, ok := props.(map[string]interface{})
	if !ok {
		t.Fatal("properties should be a map")
	}
	if _, ok := propsMap["base_branch"]; !ok {
		t.Error("Parameters() should have 'base_branch' property")
	}
}

func TestPRGeneratorTool_Execute_InvalidInput(t *testing.T) {
	tool := &PRGeneratorTool{ProjectDir: "/nonexistent"}
	ctx := context.Background()

	// Valid JSON but nonexistent directory will fail at git command.
	input := json.RawMessage(`{"base_branch": "main"}`)
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for nonexistent project directory")
	}
}

func TestPRGeneratorTool_Execute_InvalidJSON(t *testing.T) {
	tool := &PRGeneratorTool{ProjectDir: "/tmp"}
	ctx := context.Background()

	input := json.RawMessage(`{invalid json}`)
	_, err := tool.Execute(ctx, input)
	if err == nil {
		t.Error("expected error for invalid JSON input")
	}
}

func TestGenerateSummary_SingleCommit(t *testing.T) {
	commits := []CommitSummary{
		{Message: "feat(auth): add JWT validation", Type: "feat", Scope: "auth"},
	}
	summary := generateSummary(commits)
	if !strings.Contains(summary, "add JWT validation") {
		t.Errorf("single commit summary should contain stripped message, got %q", summary)
	}
}

func TestGenerateSummary_MultipleCommits(t *testing.T) {
	commits := []CommitSummary{
		{Type: "feat"},
		{Type: "feat"},
		{Type: "fix"},
	}
	summary := generateSummary(commits)
	if !strings.Contains(summary, "2 new feature(s)") {
		t.Errorf("summary should mention 2 features, got %q", summary)
	}
	if !strings.Contains(summary, "1 bug fix(es)") {
		t.Errorf("summary should mention 1 fix, got %q", summary)
	}
}

func TestGenerateSummary_Empty(t *testing.T) {
	summary := generateSummary(nil)
	if summary != "No changes." {
		t.Errorf("generateSummary(nil) = %q, want %q", summary, "No changes.")
	}
}

func TestSummarizeCommits_FewCommits(t *testing.T) {
	commits := []CommitSummary{
		{Message: "feat: add login"},
		{Message: "fix: handle error"},
	}
	result := summarizeCommits(commits)
	if !strings.Contains(result, "add login") {
		t.Errorf("should contain first commit subject, got %q", result)
	}
}

func TestSummarizeCommits_ManyCommits(t *testing.T) {
	commits := make([]CommitSummary, 5)
	for i := range commits {
		commits[i] = CommitSummary{Message: "feat: change"}
	}
	result := summarizeCommits(commits)
	if !strings.Contains(result, "5 changes") {
		t.Errorf("should summarize as '5 changes', got %q", result)
	}
}

func TestCollectBreakingChanges(t *testing.T) {
	commits := []CommitSummary{
		{Message: "feat: safe change"},
		{Message: "feat!: remove old endpoint"},
		{Message: "fix: BREAKING CHANGE: new error format"},
	}
	changes := collectBreakingChanges(commits)
	if len(changes) != 2 {
		t.Errorf("expected 2 breaking changes, got %d: %v", len(changes), changes)
	}
}

// helper
func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
