package cmd

import (
	"strings"
	"testing"
)

func TestAnalyzeDiff(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		diff       string
		wantIssues bool
	}{
		{
			"clean diff",
			`diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "log"
+func main() { log.Fatal("start") }`,
			false,
		},
		{
			"hardcoded secret",
			`diff --git a/config.go b/config.go
+const apiKey = "sk-ant-api01-real-key-here"`,
			true,
		},
		{
			"todo comment",
			`diff --git a/handler.go b/handler.go
+// TODO: fix this later`,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			findings := analyzeDiff(tt.diff)
			hasIssues := len(findings) > 0
			if hasIssues != tt.wantIssues {
				t.Errorf("analyzeDiff() found %d issues, wantIssues=%v", len(findings), tt.wantIssues)
			}
		})
	}
}

func TestCheckLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		line     string
		file     string
		wantFind bool
	}{
		{"normal code", "x := 42", "main.go", false},
		{"todo", "// TODO: fix later", "main.go", true},
		{"fixme", "// FIXME: broken", "main.go", true},
		{"hardcoded password", `password := "secret123"`, "auth.go", true},
		{"fmt.Println", `fmt.Println("debug")`, "server.go", true},
		{"empty line", "", "main.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			finding := checkLine(tt.line, tt.file, 10)
			if tt.wantFind && finding == nil {
				t.Errorf("checkLine(%q) = nil, want finding", tt.line)
			}
			if !tt.wantFind && finding != nil {
				t.Errorf("checkLine(%q) = %v, want nil", tt.line, finding)
			}
		})
	}
}

func TestFormatReview(t *testing.T) {
	t.Parallel()
	diff := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
+func main() { fmt.Println("hello") }`

	review := formatReview(diff, 42)
	if review == "" {
		t.Error("formatReview should produce non-empty output")
	}
}

func TestGenerateReviewSummary(t *testing.T) {
	t.Parallel()
	diff := "some diff content"
	findings := []finding{
		{file: "main.go", line: 10, severity: "warning", description: "potential issue"},
		{file: "config.go", line: 5, severity: "error", description: "hardcoded secret"},
	}

	summary := generateReviewSummary(diff, findings)
	if summary == "" {
		t.Error("generateReviewSummary should produce output")
	}
	if !strings.Contains(summary, "issue") && !strings.Contains(summary, "finding") && len(summary) < 10 {
		t.Error("summary should describe findings")
	}
}

func TestRequireGH(t *testing.T) {
	t.Parallel()
	err := requireGH()
	// Just verify it doesn't panic — gh may or may not be installed
	_ = err
}
