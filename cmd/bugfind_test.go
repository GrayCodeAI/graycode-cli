package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBugFindPrompt(t *testing.T) {
	dir := t.TempDir()

	// Create a test file.
	testFile := filepath.Join(dir, "handler.go")
	os.WriteFile(testFile, []byte(`package main

func handler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	db.Exec("SELECT * FROM users WHERE id = " + id)
}
`), 0o644)

	// With file paths only.
	prompt := bugFindPrompt([]string{testFile}, "")
	if !strings.Contains(prompt, "handler.go") {
		t.Error("prompt should contain the file path")
	}
	if !strings.Contains(prompt, "SELECT") {
		t.Error("prompt should contain file content")
	}
	if !strings.Contains(prompt, "Security vulnerabilities") {
		t.Error("prompt should contain analysis instructions")
	}
	if !strings.Contains(prompt, "```go") {
		t.Error("prompt should use go code fence for .go files")
	}

	// With diff content.
	diff := `--- a/handler.go
+++ b/handler.go
@@ -1,3 +1,5 @@
+func unsafe() {
+    os.Remove("/")
+}
`
	prompt = bugFindPrompt(nil, diff)
	if !strings.Contains(prompt, "Diff to analyze") {
		t.Error("prompt should contain diff section")
	}
	if !strings.Contains(prompt, "os.Remove") {
		t.Error("prompt should contain diff content")
	}
}

func TestFormatBugReport(t *testing.T) {
	findings := `1. File: handler.go, Line: 5, Severity: critical
   Description: SQL injection vulnerability
   Fix: Use parameterized queries

2. File: handler.go, Line: 3, Severity: medium
   Description: Missing input validation
   Fix: Validate the id parameter`

	report := formatBugReport(findings)
	if !strings.Contains(report, "=== Bug Report ===") {
		t.Error("report should have header")
	}
	if !strings.Contains(report, "SQL injection") {
		t.Error("report should contain finding content")
	}
	if !strings.Contains(report, "Summary") {
		t.Error("report should contain summary")
	}
	if !strings.Contains(report, "1 critical") {
		t.Error("report should count critical issues")
	}
	if !strings.Contains(report, "1 medium") {
		t.Error("report should count medium issues")
	}

	// Empty findings.
	empty := formatBugReport("")
	if !strings.Contains(empty, "No issues found") {
		t.Error("empty findings should report no issues")
	}
}
