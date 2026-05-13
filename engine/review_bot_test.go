package engine

import (
	"strings"
	"testing"
)

func TestNewReviewBot(t *testing.T) {
	bot := NewReviewBot()
	if bot == nil {
		t.Fatal("NewReviewBot returned nil")
	}
	if len(bot.Rules) < 20 {
		t.Errorf("expected at least 20 rules, got %d", len(bot.Rules))
	}
	if bot.Severity != "info" {
		t.Errorf("expected default severity 'info', got %q", bot.Severity)
	}
}

func TestReviewBotRuleCategories(t *testing.T) {
	bot := NewReviewBot()
	categories := map[string]int{}
	for _, r := range bot.Rules {
		categories[r.Category]++
	}
	expected := []string{"security", "performance", "correctness", "style", "testing"}
	for _, cat := range expected {
		if categories[cat] == 0 {
			t.Errorf("expected at least one rule in category %q", cat)
		}
	}
}

func TestReviewFileHardcodedSecret(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

var apiKey = "sk-1234567890abcdef1234"
`
	report, err := bot.ReviewFile("main.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "SEC001" {
			found = true
			if c.Severity != "error" {
				t.Errorf("expected severity 'error', got %q", c.Severity)
			}
			if c.Category != "security" {
				t.Errorf("expected category 'security', got %q", c.Category)
			}
		}
	}
	if !found {
		t.Error("expected SEC001 to trigger on hardcoded secret")
	}
}

func TestReviewFileSQLInjection(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func getUser(id string) {
	db.Query(fmt.Sprintf("SELECT * FROM users WHERE id = '%s'", id))
}
`
	report, err := bot.ReviewFile("handler.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "SEC002" {
			found = true
		}
	}
	if !found {
		t.Error("expected SEC002 to trigger on SQL injection pattern")
	}
}

func TestReviewFileCommandInjection(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

import "os/exec"

func run(userInput string) {
	exec.Command("sh", "-c", "echo " + userInput)
}
`
	report, err := bot.ReviewFile("cmd.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "SEC003" {
			found = true
		}
	}
	if !found {
		t.Error("expected SEC003 to trigger on command injection")
	}
}

func TestReviewFileXSS(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(template.HTML(r.URL.Query().Get("name"))))
}
`
	report, err := bot.ReviewFile("web.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "SEC004" {
			found = true
		}
	}
	if !found {
		t.Error("expected SEC004 to trigger on XSS pattern")
	}
}

func TestReviewFileErrorIgnored(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func process() {
	result, _ := os.Create("file.txt")
	_ = result
}
`
	report, err := bot.ReviewFile("proc.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "CORR001" {
			found = true
		}
	}
	if !found {
		t.Error("expected CORR001 to trigger on ignored error")
	}
}

func TestReviewFileUnclosedResource(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func readFile() {
	f, err := os.Open("data.txt")
	if err != nil {
		return
	}
	data := read(f)
	process(data)
}
`
	report, err := bot.ReviewFile("io.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "CORR003" {
			found = true
		}
	}
	if !found {
		t.Error("expected CORR003 to trigger on unclosed resource")
	}
}

func TestReviewFileExportedWithoutDocs(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func ExportedFunction() string {
	return "hello"
}
`
	report, err := bot.ReviewFile("api.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "STY001" {
			found = true
		}
	}
	if !found {
		t.Error("expected STY001 to trigger on undocumented exported function")
	}
}

func TestReviewFileExportedWithDocs(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

// ExportedFunction does something.
func ExportedFunction() string {
	return "hello"
}
`
	report, err := bot.ReviewFile("api.go", code)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Comments {
		if c.RuleID == "STY001" && c.Line == 4 {
			t.Error("STY001 should NOT trigger when doc comment is present")
		}
	}
}

func TestReviewFileEmptyErrorHandler(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func doSomething() {
	err := riskyOp()
	if err != nil {
	}
}
`
	report, err := bot.ReviewFile("empty.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "CORR005" {
			found = true
		}
	}
	if !found {
		t.Error("expected CORR005 to trigger on empty error handler")
	}
}

func TestReviewFileDeferInLoop(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func processFiles(paths []string) {
	for _, p := range paths {
		f, _ := os.Open(p)
		defer f.Close()
	}
}
`
	report, err := bot.ReviewFile("loop.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "CORR006" {
			found = true
		}
	}
	if !found {
		t.Error("expected CORR006 to trigger on defer inside loop")
	}
}

func TestReviewFileSkippedTest(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func TestSomething(t *testing.T) {
	t.Skip("not ready yet")
}
`
	report, err := bot.ReviewFile("foo_test.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "TEST002" {
			found = true
		}
	}
	if !found {
		t.Error("expected TEST002 to trigger on skipped test")
	}
}

func TestReviewFileTestWithoutAssertion(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func TestNoAssert(t *testing.T) {
	x := 1 + 1
	_ = x
}
`
	report, err := bot.ReviewFile("bar_test.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "TEST001" {
			found = true
		}
	}
	if !found {
		t.Error("expected TEST001 to trigger on test without assertion")
	}
}

func TestReviewFileTestFileWithoutTests(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

func helper() string {
	return "help"
}
`
	report, err := bot.ReviewFile("util_test.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "TEST003" {
			found = true
		}
	}
	if !found {
		t.Error("expected TEST003 to trigger on test file without test functions")
	}
}

func TestReviewDiff(t *testing.T) {
	bot := NewReviewBot()
	diff := `diff --git a/src/auth.go b/src/auth.go
--- a/src/auth.go
+++ b/src/auth.go
@@ -10,3 +10,5 @@ func init() {
 }

+var secret = "super_secret_password_12345"
+
 func main() {
`
	report, err := bot.ReviewDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if report.FilesReviewed != 1 {
		t.Errorf("expected 1 file reviewed, got %d", report.FilesReviewed)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "SEC001" && c.File == "src/auth.go" {
			found = true
		}
	}
	if !found {
		t.Error("expected SEC001 to trigger on hardcoded secret in diff")
	}
}

func TestReviewDiffMultipleFiles(t *testing.T) {
	bot := NewReviewBot()
	diff := `diff --git a/src/a.go b/src/a.go
--- a/src/a.go
+++ b/src/a.go
@@ -1,3 +1,5 @@
 package main

+var token = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij"
+
 func main() {}
diff --git a/src/b.go b/src/b.go
--- a/src/b.go
+++ b/src/b.go
@@ -1,3 +1,5 @@
 package main

+func ExportedNoDoc() {}
+
 func helper() {}
`
	report, err := bot.ReviewDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	if report.FilesReviewed != 2 {
		t.Errorf("expected 2 files reviewed, got %d", report.FilesReviewed)
	}
	if report.IssuesFound == 0 {
		t.Error("expected at least one issue")
	}
}

func TestReviewDiffOnlyChangedLines(t *testing.T) {
	bot := NewReviewBot()
	// The secret is in a context line (not added), so should not trigger.
	diff := `diff --git a/src/c.go b/src/c.go
--- a/src/c.go
+++ b/src/c.go
@@ -1,4 +1,5 @@
 package main

 var oldSecret = "sk-alreadyExistedBeforeThisDiff1234"
+var x = 1
 func main() {}
`
	report, err := bot.ReviewDiff(diff)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Comments {
		if c.RuleID == "SEC001" && strings.Contains(c.Message, "secret") {
			t.Error("SEC001 should not trigger on unchanged context lines")
		}
	}
}

func TestFilterBySeverity(t *testing.T) {
	comments := []ReviewComment{
		{Severity: "error", Message: "a"},
		{Severity: "warning", Message: "b"},
		{Severity: "info", Message: "c"},
		{Severity: "error", Message: "d"},
	}

	errOnly := FilterBySeverity(comments, "error")
	if len(errOnly) != 2 {
		t.Errorf("expected 2 error-level comments, got %d", len(errOnly))
	}

	warnUp := FilterBySeverity(comments, "warning")
	if len(warnUp) != 3 {
		t.Errorf("expected 3 warning+ comments, got %d", len(warnUp))
	}

	all := FilterBySeverity(comments, "info")
	if len(all) != 4 {
		t.Errorf("expected 4 info+ comments, got %d", len(all))
	}
}

func TestFormatReviewReport(t *testing.T) {
	report := &ReviewReport{
		Comments: []ReviewComment{
			{File: "src/auth.go", Line: 42, Severity: "error", Category: "security", Message: "Hardcoded secret detected", Suggestion: "Use environment variable instead"},
			{File: "src/handler.go", Line: 15, Severity: "warning", Category: "correctness", Message: "Error return value ignored", Suggestion: "if err != nil { return err }"},
			{File: "src/util.go", Line: 88, Severity: "info", Category: "style", Message: "Exported function missing documentation", Suggestion: "Add godoc comment"},
		},
		FilesReviewed: 12,
		IssuesFound:   3,
		BySeverity:    map[string]int{"error": 1, "warning": 1, "info": 1},
	}

	output := FormatReport(report)
	if !strings.Contains(output, "12 files") {
		t.Error("expected output to contain file count")
	}
	if !strings.Contains(output, "3 issues") {
		t.Error("expected output to contain issue count")
	}
	if !strings.Contains(output, "src/auth.go:42") {
		t.Error("expected output to contain file:line reference")
	}
	if !strings.Contains(output, "[security]") {
		t.Error("expected output to contain category")
	}
	if !strings.Contains(output, "Suggestion:") {
		t.Error("expected output to contain suggestion")
	}
}

func TestFormatReviewReportNil(t *testing.T) {
	output := FormatReport(nil)
	if output != "" {
		t.Error("expected empty string for nil report")
	}
}

func TestFormatInline(t *testing.T) {
	comments := []ReviewComment{
		{File: "main.go", Line: 10, Severity: "error", Category: "security", Message: "Hardcoded secret", Suggestion: "use env var"},
	}
	output := FormatInline(comments)
	if !strings.Contains(output, "### main.go:10") {
		t.Error("expected inline format with file:line header")
	}
	if !strings.Contains(output, "```suggestion") {
		t.Error("expected suggestion code block")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("expected uppercase severity")
	}
}

func TestReviewBotConcurrencySafe(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

var secret = "password_super_long_secret_123"
`
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = bot.ReviewFile("test.go", code)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestReviewBotSeverityFilter(t *testing.T) {
	bot := NewReviewBot()
	bot.Severity = "error"
	code := `package main

// This has an info-level issue: exported without docs
func Undocumented() {}
`
	report, err := bot.ReviewFile("nodoc.go", code)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Comments {
		if c.Severity == "info" {
			t.Error("info-level comment should be filtered when minimum is error")
		}
	}
}

func TestReviewFileLanguageFilter(t *testing.T) {
	bot := NewReviewBot()
	// Go-specific rule (error ignored) should not trigger on a .py file.
	code := `result, _ := someFunc()
`
	report, err := bot.ReviewFile("script.py", code)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Comments {
		if c.RuleID == "CORR001" {
			t.Error("Go-specific rule should not trigger on Python file")
		}
	}
}

func TestReviewFileHardcodedIP(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

var server = "192.168.1.100"
`
	report, err := bot.ReviewFile("config.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "SEC005" {
			found = true
		}
	}
	if !found {
		t.Error("expected SEC005 to trigger on hardcoded IP")
	}
}

func TestReviewFileLocalhost127Allowed(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

var server = "127.0.0.1"
`
	report, err := bot.ReviewFile("config.go", code)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Comments {
		if c.RuleID == "SEC005" {
			t.Error("SEC005 should not trigger on 127.0.0.1")
		}
	}
}

func TestReviewFileTODO(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

// TODO: fix this later
func broken() {}
`
	report, err := bot.ReviewFile("todo.go", code)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range report.Comments {
		if c.RuleID == "STY004" {
			found = true
		}
	}
	if !found {
		t.Error("expected STY004 to trigger on TODO comment")
	}
}

func TestReviewReportBySeverity(t *testing.T) {
	bot := NewReviewBot()
	code := `package main

var apiKey = "sk-1234567890abcdef1234"

func Undocumented() {}
`
	report, err := bot.ReviewFile("mixed.go", code)
	if err != nil {
		t.Fatal(err)
	}
	if report.BySeverity == nil {
		t.Fatal("BySeverity should not be nil")
	}
	total := report.BySeverity["error"] + report.BySeverity["warning"] + report.BySeverity["info"]
	if total != report.IssuesFound {
		t.Errorf("BySeverity sum (%d) != IssuesFound (%d)", total, report.IssuesFound)
	}
}

func TestSeverityLevel(t *testing.T) {
	if severityLevel("error") <= severityLevel("warning") {
		t.Error("error should rank higher than warning")
	}
	if severityLevel("warning") <= severityLevel("info") {
		t.Error("warning should rank higher than info")
	}
	if severityLevel("info") <= severityLevel("unknown") {
		t.Error("info should rank higher than unknown")
	}
}

func TestParseReviewDiffFiles(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package main

+var x = 1
 func main() {}
`
	files := parseReviewDiffFiles(diff)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].path != "foo.go" {
		t.Errorf("expected path 'foo.go', got %q", files[0].path)
	}
	addCount := 0
	for _, dl := range files[0].diffLines {
		if dl.Type == "add" {
			addCount++
		}
	}
	if addCount != 1 {
		t.Errorf("expected 1 added line, got %d", addCount)
	}
}

func TestMatchesLanguage(t *testing.T) {
	tests := []struct {
		file     string
		lang     string
		expected bool
	}{
		{"main.go", "go", true},
		{"main.go", "python", false},
		{"script.py", "python", true},
		{"app.js", "javascript", true},
		{"app.tsx", "javascript", true},
		{"app.java", "java", true},
		{"app.go", "java", false},
		{"any.txt", "", true},
	}
	for _, tt := range tests {
		if got := matchesLanguage(tt.file, tt.lang); got != tt.expected {
			t.Errorf("matchesLanguage(%q, %q) = %v, want %v", tt.file, tt.lang, got, tt.expected)
		}
	}
}
