package diff

import (
	"strings"
	"testing"
)

func TestNewDiffSummarizer(t *testing.T) {
	ds := NewDiffSummarizer()
	if ds == nil {
		t.Fatal("NewDiffSummarizer returned nil")
	}
}

func TestSummarizeEmpty(t *testing.T) {
	ds := NewDiffSummarizer()
	result := ds.Summarize("")
	if result == nil {
		t.Fatal("Summarize returned nil for empty input")
	}
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
	if result.OverallSummary != "No changes" {
		t.Errorf("expected 'No changes', got %q", result.OverallSummary)
	}
	if result.Impact != "low" {
		t.Errorf("expected low impact, got %s", result.Impact)
	}
}

func TestSummarizeSingleFileAdded(t *testing.T) {
	diff := `diff --git a/src/auth/token.go b/src/auth/token.go
new file mode 100644
--- /dev/null
+++ b/src/auth/token.go
@@ -0,0 +1,15 @@
+package auth
+
+import "errors"
+
+type Claims struct {
+	UserID string
+	Exp    int64
+}
+
+func ValidateToken(token string) (*Claims, error) {
+	if token == "" {
+		return nil, errors.New("empty token")
+	}
+	return &Claims{UserID: "user1"}, nil
+}
`
	ds := NewDiffSummarizer()
	result := ds.Summarize(diff)

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	f := result.Files[0]
	if f.Path != "src/auth/token.go" {
		t.Errorf("expected path src/auth/token.go, got %s", f.Path)
	}
	if f.Action != "added" {
		t.Errorf("expected action added, got %s", f.Action)
	}
	if f.Additions != 15 {
		t.Errorf("expected 15 additions, got %d", f.Additions)
	}
	if f.Deletions != 0 {
		t.Errorf("expected 0 deletions, got %d", f.Deletions)
	}

	// Should detect new function and struct
	foundFunc := false
	foundStruct := false
	for _, kc := range f.KeyChanges {
		if strings.Contains(kc, "ValidateToken") {
			foundFunc = true
		}
		if strings.Contains(kc, "Claims") {
			foundStruct = true
		}
	}
	if !foundFunc {
		t.Error("expected to detect new ValidateToken function")
	}
	if !foundStruct {
		t.Error("expected to detect new Claims struct")
	}
}

func TestSummarizeModifiedFile(t *testing.T) {
	diff := `diff --git a/handler/api.go b/handler/api.go
--- a/handler/api.go
+++ b/handler/api.go
@@ -5,6 +5,8 @@ import "net/http"

 func SetupRoutes(mux *http.ServeMux) {
+	mux.Handle("/api/", AuthMiddleware(apiHandler))
+	mux.Handle("/health", healthHandler)
-	mux.Handle("/api/", apiHandler)
 }
`
	ds := NewDiffSummarizer()
	result := ds.Summarize(diff)

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	f := result.Files[0]
	if f.Action != "modified" {
		t.Errorf("expected modified, got %s", f.Action)
	}
	if f.Additions != 2 {
		t.Errorf("expected 2 additions, got %d", f.Additions)
	}
	if f.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", f.Deletions)
	}
}

func TestSummarizeDeletedFile(t *testing.T) {
	diff := `diff --git a/old/legacy.go b/old/legacy.go
deleted file mode 100644
--- a/old/legacy.go
+++ /dev/null
@@ -1,5 +0,0 @@
-package old
-
-func Deprecated() {
-	// do nothing
-}
`
	ds := NewDiffSummarizer()
	result := ds.Summarize(diff)

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}

	f := result.Files[0]
	if f.Action != "deleted" {
		t.Errorf("expected deleted, got %s", f.Action)
	}
	if f.Deletions != 5 {
		t.Errorf("expected 5 deletions, got %d", f.Deletions)
	}
}

func TestSummarizeMultipleFiles(t *testing.T) {
	diff := `diff --git a/src/auth/token.go b/src/auth/token.go
new file mode 100644
--- /dev/null
+++ b/src/auth/token.go
@@ -0,0 +1,10 @@
+package auth
+
+func ValidateToken(token string) (*Claims, error) {
+	if token == "" {
+		return nil, ErrEmptyToken
+	}
+	return parseClaims(token)
+}
+
+type Claims struct{ UserID string }
diff --git a/src/handler/api.go b/src/handler/api.go
--- a/src/handler/api.go
+++ b/src/handler/api.go
@@ -10,3 +10,5 @@ func SetupRoutes() {
+	wrapped := AuthMiddleware(handler)
+	mux.Handle("/api/", wrapped)
-	mux.Handle("/api/", handler)
diff --git a/src/auth/token_test.go b/src/auth/token_test.go
new file mode 100644
--- /dev/null
+++ b/src/auth/token_test.go
@@ -0,0 +1,8 @@
+package auth
+
+import "testing"
+
+func TestValidateToken(t *testing.T) {
+	_, err := ValidateToken("")
+	if err == nil { t.Fatal("expected error") }
+}
`
	ds := NewDiffSummarizer()
	result := ds.Summarize(diff)

	if len(result.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(result.Files))
	}

	// Check affected areas
	if len(result.AffectedAreas) < 2 {
		t.Errorf("expected at least 2 affected areas, got %d: %v", len(result.AffectedAreas), result.AffectedAreas)
	}

	// Should detect as feature since new files with new functions
	if result.ChangeType != "feature" {
		t.Errorf("expected change type feature, got %s", result.ChangeType)
	}
}

func TestDetectChangeTypeTest(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "auth/token_test.go", Action: "added"},
			{Path: "handler/api_test.go", Action: "modified"},
		},
	}
	got := ds.DetectChangeType(summary)
	if got != "test" {
		t.Errorf("expected test, got %s", got)
	}
}

func TestDetectChangeTypeDocs(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "docs/README.md", Action: "modified"},
			{Path: "docs/setup.md", Action: "added"},
		},
	}
	got := ds.DetectChangeType(summary)
	if got != "docs" {
		t.Errorf("expected docs, got %s", got)
	}
}

func TestDetectChangeTypeConfig(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "config.yaml", Action: "modified"},
			{Path: ".gitignore", Action: "modified"},
		},
	}
	got := ds.DetectChangeType(summary)
	if got != "config" {
		t.Errorf("expected config, got %s", got)
	}
}

func TestDetectChangeTypeBugfix(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "handler/api.go", Action: "modified", Summary: "Add error handling"},
		},
	}
	got := ds.DetectChangeType(summary)
	if got != "bugfix" {
		t.Errorf("expected bugfix, got %s", got)
	}
}

func TestDetectChangeTypeFeature(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "auth/token.go", Action: "added", KeyChanges: []string{"new ValidateToken()"}},
		},
	}
	got := ds.DetectChangeType(summary)
	if got != "feature" {
		t.Errorf("expected feature, got %s", got)
	}
}

func TestAssessImpactLow(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "internal/util.go", Additions: 5, Deletions: 2, KeyChanges: []string{"new helper()"}},
		},
	}
	got := ds.AssessImpact(summary)
	if got != "low" {
		t.Errorf("expected low, got %s", got)
	}
}

func TestAssessImpactMedium(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "auth/token.go", Additions: 20, Deletions: 5, KeyChanges: []string{"new ValidateToken()"}},
		},
	}
	got := ds.AssessImpact(summary)
	if got != "medium" {
		t.Errorf("expected medium, got %s", got)
	}
}

func TestAssessImpactHigh(t *testing.T) {
	ds := NewDiffSummarizer()
	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "auth/token.go", Additions: 50, Deletions: 10, KeyChanges: []string{"new ValidateToken()"}},
			{Path: "handler/api.go", Additions: 30, Deletions: 5, KeyChanges: []string{"new SetupAuth()"}},
			{Path: "model/user.go", Additions: 40, Deletions: 0, KeyChanges: []string{"new User struct"}},
			{Path: "config/auth.go", Additions: 20, Deletions: 0, KeyChanges: []string{"new Config struct"}},
			{Path: "middleware/jwt.go", Additions: 60, Deletions: 0, KeyChanges: []string{"new JWTMiddleware()"}},
		},
	}
	got := ds.AssessImpact(summary)
	if got != "high" {
		t.Errorf("expected high, got %s", got)
	}
}

func TestGenerateCommitMessage(t *testing.T) {
	ds := NewDiffSummarizer()

	tests := []struct {
		name     string
		summary  *DiffSummary
		contains string
	}{
		{
			name:     "nil summary",
			summary:  nil,
			contains: "chore: update code",
		},
		{
			name: "feature with scope",
			summary: &DiffSummary{
				Files:          []FileSummary{{Path: "auth/token.go", Action: "added"}},
				ChangeType:     "feature",
				AffectedAreas:  []string{"auth", "handler"},
				OverallSummary: "Add JWT token validation",
			},
			contains: "feat(auth):",
		},
		{
			name: "bugfix",
			summary: &DiffSummary{
				Files:          []FileSummary{{Path: "handler/api.go", Action: "modified"}},
				ChangeType:     "bugfix",
				AffectedAreas:  []string{"handler"},
				OverallSummary: "Fix nil pointer in request handler",
			},
			contains: "fix(handler):",
		},
		{
			name: "test",
			summary: &DiffSummary{
				Files:          []FileSummary{{Path: "auth/token_test.go", Action: "added"}},
				ChangeType:     "test",
				AffectedAreas:  []string{"auth"},
				OverallSummary: "Add unit tests for token validation",
			},
			contains: "test(auth):",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := ds.GenerateCommitMessage(tt.summary)
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("expected commit message to contain %q, got %q", tt.contains, msg)
			}
		})
	}
}

func TestGeneratePRSummary(t *testing.T) {
	ds := NewDiffSummarizer()

	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "src/auth/token.go", Action: "added", Summary: "Add ValidateToken", KeyChanges: []string{"new ValidateToken()", "new Claims struct"}},
			{Path: "src/handler/api.go", Action: "modified", Summary: "Add auth middleware call", KeyChanges: []string{}},
		},
		OverallSummary: "Added JWT authentication with validation and tests",
		ChangeType:     "feature",
		Impact:         "medium",
		AffectedAreas:  []string{"auth", "handler"},
	}

	pr := ds.GeneratePRSummary(summary)

	if !strings.Contains(pr, "## Summary") {
		t.Error("expected PR to contain Summary header")
	}
	if !strings.Contains(pr, "## Changes") {
		t.Error("expected PR to contain Changes header")
	}
	if !strings.Contains(pr, "feature") {
		t.Error("expected PR to contain change type")
	}
	if !strings.Contains(pr, "medium") {
		t.Error("expected PR to contain impact level")
	}
	if !strings.Contains(pr, "src/auth/token.go") {
		t.Error("expected PR to mention file path")
	}
	if !strings.Contains(pr, "## Affected Areas") {
		t.Error("expected PR to contain Affected Areas section")
	}
}

func TestGeneratePRSummaryEmpty(t *testing.T) {
	ds := NewDiffSummarizer()
	pr := ds.GeneratePRSummary(nil)
	if pr != "No changes to summarize." {
		t.Errorf("expected no changes message, got %q", pr)
	}
}

func TestDiffSummarizerFormatSummary(t *testing.T) {
	ds := NewDiffSummarizer()

	summary := &DiffSummary{
		Files: []FileSummary{
			{Path: "src/auth/token.go", Action: "added", Summary: "Add JWT validation function", KeyChanges: []string{"new ValidateToken()", "new Claims struct"}},
			{Path: "src/handler/api.go", Action: "modified", Summary: "Add auth middleware call", KeyChanges: []string{"wrapped routes with AuthMiddleware"}},
			{Path: "src/auth/token_test.go", Action: "added", Summary: "Tests for token validation", KeyChanges: []string{}},
		},
		OverallSummary: "Added JWT authentication with validation and tests",
		ChangeType:     "feature",
		Impact:         "medium",
		AffectedAreas:  []string{"auth", "handler"},
	}

	output := ds.FormatSummary(summary)

	if !strings.Contains(output, "Diff Summary:") {
		t.Error("expected output to start with 'Diff Summary:'")
	}
	if !strings.Contains(output, "Type: feature | Impact: medium") {
		t.Error("expected type and impact line")
	}
	if !strings.Contains(output, "+ src/auth/token.go") {
		t.Error("expected added file with + prefix")
	}
	if !strings.Contains(output, "~ src/handler/api.go") {
		t.Error("expected modified file with ~ prefix")
	}
	if !strings.Contains(output, "Key:") {
		t.Error("expected Key: line for files with key changes")
	}
	if !strings.Contains(output, "Overall:") {
		t.Error("expected Overall: line")
	}
	if !strings.Contains(output, "Affected: auth, handler") {
		t.Error("expected Affected: line")
	}
}

func TestDiffSummarizerFormatSummaryEmpty(t *testing.T) {
	ds := NewDiffSummarizer()
	output := ds.FormatSummary(nil)
	if !strings.Contains(output, "No changes detected") {
		t.Errorf("expected no changes message, got %q", output)
	}
}

func TestSummarizeFileKeyChanges(t *testing.T) {
	ds := NewDiffSummarizer()

	hunks := []string{
		`@@ -0,0 +1,20 @@
+package auth
+
+type TokenValidator interface {
+	Validate(token string) error
+}
+
+type Claims struct {
+	UserID string
+	Role   string
+}
+
+func NewValidator() *Validator {
+	return &Validator{}
+}
+
+func (v *Validator) Validate(token string) error {
+	return nil
+}
`,
	}

	fs := ds.SummarizeFile("auth/validator.go", hunks)

	if fs.Additions != 18 {
		t.Errorf("expected 18 additions, got %d", fs.Additions)
	}

	foundInterface := false
	foundStruct := false
	foundFunc := false
	for _, kc := range fs.KeyChanges {
		if strings.Contains(kc, "TokenValidator") && strings.Contains(kc, "interface") {
			foundInterface = true
		}
		if strings.Contains(kc, "Claims") && strings.Contains(kc, "struct") {
			foundStruct = true
		}
		if strings.Contains(kc, "NewValidator") || strings.Contains(kc, "Validate") {
			foundFunc = true
		}
	}

	if !foundInterface {
		t.Error("expected to detect TokenValidator interface")
	}
	if !foundStruct {
		t.Error("expected to detect Claims struct")
	}
	if !foundFunc {
		t.Error("expected to detect new functions")
	}
}

func TestExtractAffectedAreas(t *testing.T) {
	ds := NewDiffSummarizer()
	files := []FileSummary{
		{Path: "src/auth/token.go"},
		{Path: "src/auth/claims.go"},
		{Path: "src/handler/api.go"},
		{Path: "config.yaml"},
	}

	areas := ds.extractAffectedAreas(files)

	// Should deduplicate auth
	authCount := 0
	for _, a := range areas {
		if a == "auth" {
			authCount++
		}
	}
	if authCount != 1 {
		t.Errorf("expected auth to appear exactly once, got %d", authCount)
	}

	if len(areas) < 2 {
		t.Errorf("expected at least 2 unique areas, got %d: %v", len(areas), areas)
	}
}

func TestDiffSumIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"auth/token_test.go", true},
		{"src/handler/api_test.go", true},
		{"tests/integration.go", true},
		{"__tests__/app.test.js", true},
		{"auth/token.go", false},
		{"main.go", false},
	}

	for _, tt := range tests {
		got := diffSumIsTestFile(tt.path)
		if got != tt.expected {
			t.Errorf("diffSumIsTestFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestDiffSumIsDocFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"README.md", true},
		{"docs/setup.rst", true},
		{"CHANGELOG.txt", true},
		{"main.go", false},
	}

	for _, tt := range tests {
		got := diffSumIsDocFile(tt.path)
		if got != tt.expected {
			t.Errorf("diffSumIsDocFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestDiffSumIsConfigFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"config.yaml", true},
		{"docker-compose.yml", true},
		{".gitignore", true},
		{"go.mod", true},
		{"Dockerfile", true},
		{"main.go", false},
	}

	for _, tt := range tests {
		got := diffSumIsConfigFile(tt.path)
		if got != tt.expected {
			t.Errorf("diffSumIsConfigFile(%q) = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestEndToEndSummarize(t *testing.T) {
	diff := `diff --git a/src/auth/token.go b/src/auth/token.go
new file mode 100644
--- /dev/null
+++ b/src/auth/token.go
@@ -0,0 +1,15 @@
+package auth
+
+import "errors"
+
+type Claims struct {
+	UserID string
+	Exp    int64
+}
+
+func ValidateToken(token string) (*Claims, error) {
+	if token == "" {
+		return nil, errors.New("empty token")
+	}
+	return &Claims{UserID: "user1"}, nil
+}
diff --git a/src/handler/api.go b/src/handler/api.go
--- a/src/handler/api.go
+++ b/src/handler/api.go
@@ -8,3 +8,5 @@ func SetupRoutes(mux *http.ServeMux) {
+	wrapped := AuthMiddleware(apiHandler)
+	mux.Handle("/api/", wrapped)
-	mux.Handle("/api/", apiHandler)
diff --git a/src/auth/token_test.go b/src/auth/token_test.go
new file mode 100644
--- /dev/null
+++ b/src/auth/token_test.go
@@ -0,0 +1,10 @@
+package auth
+
+import "testing"
+
+func TestValidateToken(t *testing.T) {
+	_, err := ValidateToken("")
+	if err == nil {
+		t.Fatal("expected error for empty token")
+	}
+}
`
	ds := NewDiffSummarizer()
	result := ds.Summarize(diff)

	// Verify structure
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(result.Files))
	}

	// Verify change type is feature (new files with new functions)
	if result.ChangeType != "feature" {
		t.Errorf("expected feature, got %s", result.ChangeType)
	}

	// Verify FormatSummary works
	formatted := ds.FormatSummary(result)
	if formatted == "" {
		t.Error("FormatSummary returned empty string")
	}

	// Verify commit message
	commitMsg := ds.GenerateCommitMessage(result)
	if !strings.HasPrefix(commitMsg, "feat") {
		t.Errorf("expected commit message to start with feat, got %q", commitMsg)
	}

	// Verify PR summary
	prSummary := ds.GeneratePRSummary(result)
	if !strings.Contains(prSummary, "## Summary") {
		t.Error("PR summary missing Summary section")
	}
}
