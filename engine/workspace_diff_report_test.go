package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewDiffReporter(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")
	if dr == nil {
		t.Fatal("NewDiffReporter returned nil")
	}
	if dr.ProjectDir != "/tmp/project" {
		t.Errorf("ProjectDir = %q, want %q", dr.ProjectDir, "/tmp/project")
	}
}

func TestGenerateReport_Empty(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")
	report := dr.GenerateReport(map[string]string{})

	if report == nil {
		t.Fatal("GenerateReport returned nil")
	}
	if len(report.Files) != 0 {
		t.Errorf("Files count = %d, want 0", len(report.Files))
	}
	if report.TotalAdditions != 0 {
		t.Errorf("TotalAdditions = %d, want 0", report.TotalAdditions)
	}
	if report.TotalDeletions != 0 {
		t.Errorf("TotalDeletions = %d, want 0", report.TotalDeletions)
	}
	if report.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should not be zero")
	}
}

func TestGenerateReport_SingleModifiedFile(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")

	diff := `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -10,6 +10,8 @@ func main() {
 	fmt.Println("hello")
+	fmt.Println("world")
+	fmt.Println("!")
-	fmt.Println("old")
 }
`

	report := dr.GenerateReport(map[string]string{
		"main.go": diff,
	})

	if len(report.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(report.Files))
	}

	f := report.Files[0]
	if f.Path != "main.go" {
		t.Errorf("Path = %q, want %q", f.Path, "main.go")
	}
	if f.Status != "modified" {
		t.Errorf("Status = %q, want %q", f.Status, "modified")
	}
	if f.Additions != 2 {
		t.Errorf("Additions = %d, want 2", f.Additions)
	}
	if f.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1", f.Deletions)
	}
	if report.TotalAdditions != 2 {
		t.Errorf("TotalAdditions = %d, want 2", report.TotalAdditions)
	}
	if report.TotalDeletions != 1 {
		t.Errorf("TotalDeletions = %d, want 1", report.TotalDeletions)
	}
	if report.FilesModified != 1 {
		t.Errorf("FilesModified = %d, want 1", report.FilesModified)
	}
}

func TestGenerateReport_NewFile(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")

	diff := `diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/new.go
@@ -0,0 +1,10 @@
+package main
+
+func NewHandler() *Handler {
+	return &Handler{}
+}
+
+type Handler struct {
+	Name string
+}
`

	report := dr.GenerateReport(map[string]string{
		"new.go": diff,
	})

	if len(report.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(report.Files))
	}

	f := report.Files[0]
	if f.Status != "added" {
		t.Errorf("Status = %q, want %q", f.Status, "added")
	}
	if f.Additions != 9 {
		t.Errorf("Additions = %d, want 9", f.Additions)
	}
	if report.FilesAdded != 1 {
		t.Errorf("FilesAdded = %d, want 1", report.FilesAdded)
	}

	// Should detect key changes
	if len(f.KeyChanges) == 0 {
		t.Error("Expected key changes to be detected")
	}

	foundFunc := false
	foundStruct := false
	for _, kc := range f.KeyChanges {
		if strings.Contains(kc, "NewHandler") {
			foundFunc = true
		}
		if strings.Contains(kc, "Handler") && strings.Contains(kc, "struct") {
			foundStruct = true
		}
	}
	if !foundFunc {
		t.Error("Expected key change for NewHandler function")
	}
	if !foundStruct {
		t.Error("Expected key change for Handler struct")
	}
}

func TestGenerateReport_DeletedFile(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")

	diff := `diff --git a/old.go b/old.go
deleted file mode 100644
index abc1234..0000000
--- a/old.go
+++ /dev/null
@@ -1,5 +0,0 @@
-package main
-
-func OldFunc() {
-	// deprecated
-}
`

	report := dr.GenerateReport(map[string]string{
		"old.go": diff,
	})

	if len(report.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(report.Files))
	}

	f := report.Files[0]
	if f.Status != "deleted" {
		t.Errorf("Status = %q, want %q", f.Status, "deleted")
	}
	if f.Deletions != 5 {
		t.Errorf("Deletions = %d, want 5", f.Deletions)
	}
	if report.FilesDeleted != 1 {
		t.Errorf("FilesDeleted = %d, want 1", report.FilesDeleted)
	}
}

func TestGenerateReport_MultipleFiles(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")

	files := map[string]string{
		"a.go": `--- /dev/null
+++ b/a.go
+package main
+func A() {}
`,
		"b.go": `--- a/b.go
+++ b/b.go
+func Updated() {}
-func Old() {}
`,
		"c.go": `deleted file mode 100644
--- a/c.go
+++ /dev/null
-package main
-func C() {}
`,
	}

	report := dr.GenerateReport(files)

	if len(report.Files) != 3 {
		t.Fatalf("Files count = %d, want 3", len(report.Files))
	}
	if report.FilesAdded != 1 {
		t.Errorf("FilesAdded = %d, want 1", report.FilesAdded)
	}
	if report.FilesModified != 1 {
		t.Errorf("FilesModified = %d, want 1", report.FilesModified)
	}
	if report.FilesDeleted != 1 {
		t.Errorf("FilesDeleted = %d, want 1", report.FilesDeleted)
	}

	// Verify deterministic order (alphabetical)
	if report.Files[0].Path != "a.go" {
		t.Errorf("First file = %q, want %q", report.Files[0].Path, "a.go")
	}
	if report.Files[1].Path != "b.go" {
		t.Errorf("Second file = %q, want %q", report.Files[1].Path, "b.go")
	}
	if report.Files[2].Path != "c.go" {
		t.Errorf("Third file = %q, want %q", report.Files[2].Path, "c.go")
	}
}

func TestFormatAsMarkdown_Empty(t *testing.T) {
	result := FormatAsMarkdown(nil)
	if !strings.Contains(result, "No changes detected") {
		t.Error("Expected 'No changes detected' for nil report")
	}

	result = FormatAsMarkdown(&WorkspaceDiffReport{Files: []FileDiffReport{}})
	if !strings.Contains(result, "No changes detected") {
		t.Error("Expected 'No changes detected' for empty report")
	}
}

func TestFormatAsMarkdown_FullReport(t *testing.T) {
	report := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{
				Path:       "src/auth/token.go",
				Status:     "modified",
				Additions:  15,
				Deletions:  3,
				Summary:    "Added token validation",
				KeyChanges: []string{"Added ValidateToken()"},
			},
			{
				Path:       "src/middleware.go",
				Status:     "added",
				Additions:  45,
				Deletions:  0,
				Summary:    "New rate limiting middleware",
				KeyChanges: []string{"Added RateLimiter struct"},
			},
			{
				Path:       "src/old_handler.go",
				Status:     "deleted",
				Additions:  0,
				Deletions:  30,
				Summary:    "Removed deprecated handler",
				KeyChanges: []string{"Removed LegacyHandler()"},
			},
		},
		TotalAdditions:  60,
		TotalDeletions:  33,
		FilesAdded:      1,
		FilesModified:   1,
		FilesDeleted:    1,
		SessionDuration: 12*time.Minute + 30*time.Second,
		GeneratedAt:     time.Now(),
	}

	result := FormatAsMarkdown(report)

	// Check header
	if !strings.Contains(result, "## Session Changes") {
		t.Error("Missing session changes header")
	}

	// Check summary line
	if !strings.Contains(result, "1 files modified") {
		t.Error("Missing modified count in summary")
	}
	if !strings.Contains(result, "1 added") {
		t.Error("Missing added count in summary")
	}
	if !strings.Contains(result, "1 deleted") {
		t.Error("Missing deleted count in summary")
	}
	if !strings.Contains(result, "+60, -33") {
		t.Error("Missing total additions/deletions")
	}

	// Check duration
	if !strings.Contains(result, "12m 30s") {
		t.Error("Missing duration")
	}

	// Check table
	if !strings.Contains(result, "| File | Status | Changes | Summary |") {
		t.Error("Missing table header")
	}
	if !strings.Contains(result, "src/auth/token.go") {
		t.Error("Missing file path in table")
	}
	if !strings.Contains(result, "modified") {
		t.Error("Missing status in table")
	}

	// Check key changes
	if !strings.Contains(result, "### Key Changes") {
		t.Error("Missing key changes section")
	}
	if !strings.Contains(result, "Added ValidateToken()") {
		t.Error("Missing key change entry")
	}
}

func TestFormatAsTerminal_Empty(t *testing.T) {
	result := FormatAsTerminal(nil)
	if !strings.Contains(result, "No changes detected") {
		t.Error("Expected 'No changes detected' for nil report")
	}
}

func TestFormatAsTerminal_WithFiles(t *testing.T) {
	report := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{
				Path:       "main.go",
				Status:     "modified",
				Additions:  10,
				Deletions:  5,
				Summary:    "Updated main logic",
				KeyChanges: []string{"Added Run()"},
			},
			{
				Path:       "new_file.go",
				Status:     "added",
				Additions:  20,
				Deletions:  0,
				Summary:    "New helper functions",
				KeyChanges: []string{},
			},
		},
		TotalAdditions:  30,
		TotalDeletions:  5,
		FilesAdded:      1,
		FilesModified:   1,
		SessionDuration: 5 * time.Minute,
		GeneratedAt:     time.Now(),
	}

	result := FormatAsTerminal(report)

	// Should contain ANSI color codes
	if !strings.Contains(result, "\033[") {
		t.Error("Expected ANSI color codes in terminal output")
	}

	// Should contain file names
	if !strings.Contains(result, "main.go") {
		t.Error("Missing main.go in output")
	}
	if !strings.Contains(result, "new_file.go") {
		t.Error("Missing new_file.go in output")
	}

	// Should have session header
	if !strings.Contains(result, "Session Changes") {
		t.Error("Missing session header")
	}

	// Should contain key changes
	if !strings.Contains(result, "Key Changes") {
		t.Error("Missing key changes section")
	}
	if !strings.Contains(result, "Added Run()") {
		t.Error("Missing key change")
	}
}

func TestFormatForCommit_Empty(t *testing.T) {
	result := FormatForCommit(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil report, got %q", result)
	}

	result = FormatForCommit(&WorkspaceDiffReport{Files: []FileDiffReport{}})
	if result != "" {
		t.Errorf("Expected empty string for empty report, got %q", result)
	}
}

func TestFormatForCommit_WithFiles(t *testing.T) {
	report := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{
				Path:       "handler.go",
				Status:     "modified",
				Additions:  8,
				Deletions:  2,
				Summary:    "Updated error handling",
				KeyChanges: []string{"Added HandleError()"},
			},
			{
				Path:       "util.go",
				Status:     "added",
				Additions:  15,
				Deletions:  0,
				Summary:    "New utility functions",
				KeyChanges: []string{},
			},
		},
		TotalAdditions: 23,
		TotalDeletions: 2,
		FilesAdded:     1,
		FilesModified:  1,
		GeneratedAt:    time.Now(),
	}

	result := FormatForCommit(report)

	// Check summary line
	if !strings.Contains(result, "2 file(s) (+23/-2)") {
		t.Errorf("Missing summary line, got:\n%s", result)
	}

	// Check file status indicators
	if !strings.Contains(result, "M handler.go") {
		t.Error("Missing modified file indicator")
	}
	if !strings.Contains(result, "A util.go") {
		t.Error("Missing added file indicator")
	}

	// Check highlights
	if !strings.Contains(result, "Highlights:") {
		t.Error("Missing highlights section")
	}
	if !strings.Contains(result, "Added HandleError()") {
		t.Error("Missing key change in highlights")
	}
}

func TestCompareReports_BothNil(t *testing.T) {
	result := CompareReports(nil, nil)
	if !strings.Contains(result, "No reports to compare") {
		t.Errorf("Expected 'No reports to compare', got %q", result)
	}
}

func TestCompareReports_BeforeNil(t *testing.T) {
	after := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{Path: "a.go", Status: "added", Additions: 10},
		},
		TotalAdditions: 10,
	}

	result := CompareReports(nil, after)
	if !strings.Contains(result, "New session") {
		t.Error("Expected 'New session' message")
	}
	if !strings.Contains(result, "1 file(s) changed") {
		t.Error("Expected file count")
	}
}

func TestCompareReports_AfterNil(t *testing.T) {
	before := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{Path: "a.go", Status: "modified", Additions: 5},
		},
		TotalAdditions: 5,
	}

	result := CompareReports(before, nil)
	if !strings.Contains(result, "Session ended with no final report") {
		t.Error("Expected session ended message")
	}
}

func TestCompareReports_WithChanges(t *testing.T) {
	before := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{Path: "a.go", Status: "modified", Additions: 5, Deletions: 2},
			{Path: "b.go", Status: "added", Additions: 10, Deletions: 0},
		},
		TotalAdditions: 15,
		TotalDeletions: 2,
	}

	after := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{Path: "a.go", Status: "modified", Additions: 8, Deletions: 3},
			{Path: "c.go", Status: "added", Additions: 20, Deletions: 0},
		},
		TotalAdditions: 28,
		TotalDeletions: 3,
	}

	result := CompareReports(before, after)

	// Should show report comparison header
	if !strings.Contains(result, "## Report Comparison") {
		t.Error("Missing comparison header")
	}

	// Should show delta for files
	if !strings.Contains(result, "Files: 2 -> 2") {
		t.Error("Missing file count comparison")
	}

	// Should show new files
	if !strings.Contains(result, "c.go") {
		t.Error("Missing new file c.go")
	}

	// Should show removed files
	if !strings.Contains(result, "b.go") {
		t.Error("Missing removed file b.go")
	}

	// Should show changed files
	if !strings.Contains(result, "a.go") {
		t.Error("Missing changed file a.go")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{12*time.Minute + 30*time.Second, "12m 30s"},
		{1 * time.Hour, "1h"},
		{1*time.Hour + 15*time.Minute, "1h 15m"},
		{2*time.Hour + 30*time.Minute, "2h 30m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatChangeCounts(t *testing.T) {
	tests := []struct {
		adds int
		dels int
		want string
	}{
		{10, 5, "+10, -5"},
		{10, 0, "+10"},
		{0, 5, "-5"},
		{0, 0, "0"},
	}

	for _, tt := range tests {
		got := formatChangeCounts(tt.adds, tt.dels)
		if got != tt.want {
			t.Errorf("formatChangeCounts(%d, %d) = %q, want %q", tt.adds, tt.dels, got, tt.want)
		}
	}
}

func TestFormatDelta(t *testing.T) {
	tests := []struct {
		delta int
		want  string
	}{
		{5, "+5"},
		{-3, "-3"},
		{0, "0"},
	}

	for _, tt := range tests {
		got := formatDelta(tt.delta)
		if got != tt.want {
			t.Errorf("formatDelta(%d) = %q, want %q", tt.delta, got, tt.want)
		}
	}
}

func TestSplitIntoDiffs(t *testing.T) {
	fullDiff := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {}
diff --git a/util.go b/util.go
new file mode 100644
index 0000000..abc1234
--- /dev/null
+++ b/util.go
@@ -0,0 +1,3 @@
+package main
+
+func Util() {}
`

	result := splitIntoDiffs(fullDiff)

	if len(result) != 2 {
		t.Fatalf("Got %d files, want 2", len(result))
	}

	if _, ok := result["main.go"]; !ok {
		t.Error("Missing main.go in result")
	}
	if _, ok := result["util.go"]; !ok {
		t.Error("Missing util.go in result")
	}
}

func TestParseStatCounts(t *testing.T) {
	tests := []struct {
		stat    string
		wantAdd int
		wantDel int
	}{
		{"5 ++---", 2, 3},
		{"10 ++++++++++", 10, 0},
		{"3 ---", 0, 3},
		{"8 ++++----", 4, 4},
		{"0", 0, 0},
		{"", 0, 0},
	}

	for _, tt := range tests {
		gotAdd, gotDel := parseStatCounts(tt.stat)
		if gotAdd != tt.wantAdd || gotDel != tt.wantDel {
			t.Errorf("parseStatCounts(%q) = (%d, %d), want (%d, %d)",
				tt.stat, gotAdd, gotDel, tt.wantAdd, tt.wantDel)
		}
	}
}

func TestDetectReportKeyChanges(t *testing.T) {
	added := []string{
		"func NewService() *Service {",
		"	return &Service{}",
		"}",
		"type Config struct {",
		"	Host string",
		"}",
	}
	removed := []string{
		"func OldService() *Service {",
		"	return nil",
		"}",
	}

	changes := detectReportKeyChanges(added, removed)

	if len(changes) == 0 {
		t.Fatal("Expected key changes to be detected")
	}

	foundNewFunc := false
	foundRemovedFunc := false
	foundNewStruct := false

	for _, c := range changes {
		if strings.Contains(c, "Added NewService()") {
			foundNewFunc = true
		}
		if strings.Contains(c, "Removed OldService()") {
			foundRemovedFunc = true
		}
		if strings.Contains(c, "Added Config struct") {
			foundNewStruct = true
		}
	}

	if !foundNewFunc {
		t.Error("Expected 'Added NewService()' in key changes")
	}
	if !foundRemovedFunc {
		t.Error("Expected 'Removed OldService()' in key changes")
	}
	if !foundNewStruct {
		t.Error("Expected 'Added Config struct' in key changes")
	}
}

func TestGenerateReportSummary(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		added   []string
		removed []string
		expect  string // substring expected in result
	}{
		{
			name:    "new function",
			path:    "service.go",
			added:   []string{"func Handle() {", "  return", "}"},
			removed: nil,
			expect:  "Handle",
		},
		{
			name:    "new struct",
			path:    "types.go",
			added:   []string{"type Config struct {", "  Port int", "}"},
			removed: nil,
			expect:  "Config",
		},
		{
			name:    "pure deletion",
			path:    "old.go",
			added:   nil,
			removed: []string{"func Old() {", "  return", "}"},
			expect:  "Old",
		},
		{
			name:    "mixed changes no functions",
			path:    "config.yaml",
			added:   []string{"port: 8080", "host: localhost"},
			removed: []string{"port: 3000"},
			expect:  "Modified",
		},
		{
			name:    "empty changes",
			path:    "empty.go",
			added:   nil,
			removed: nil,
			expect:  "No significant changes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateReportSummary(tt.path, tt.added, tt.removed)
			if !strings.Contains(result, tt.expect) {
				t.Errorf("generateReportSummary() = %q, expected to contain %q", result, tt.expect)
			}
		})
	}
}

func TestRenderChangeBar(t *testing.T) {
	// Basic test that it produces output of expected structure
	bar := renderChangeBar(10, 5, 20)
	if !strings.Contains(bar, "+") {
		t.Error("Expected + characters in bar")
	}
	if !strings.Contains(bar, "-") {
		t.Error("Expected - characters in bar")
	}

	// Zero case
	bar = renderChangeBar(0, 0, 20)
	if strings.Contains(bar, "+") || strings.Contains(bar, "-") {
		t.Error("Expected empty bar for zero changes")
	}

	// Only additions
	bar = renderChangeBar(10, 0, 20)
	if !strings.Contains(bar, "+") {
		t.Error("Expected + in additions-only bar")
	}

	// Only deletions
	bar = renderChangeBar(0, 10, 20)
	if !strings.Contains(bar, "-") {
		t.Error("Expected - in deletions-only bar")
	}
}

func TestTruncateSlice(t *testing.T) {
	s := []string{"a", "b", "c", "d", "e"}

	result := truncateSlice(s, 3)
	if len(result) != 3 {
		t.Errorf("Got %d elements, want 3", len(result))
	}

	result = truncateSlice(s, 10)
	if len(result) != 5 {
		t.Errorf("Got %d elements, want 5 (original length)", len(result))
	}

	result = truncateSlice(nil, 3)
	if len(result) != 0 {
		t.Errorf("Got %d elements, want 0 for nil slice", len(result))
	}
}

func TestFormatAsMarkdown_NoDuration(t *testing.T) {
	report := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{
				Path:       "main.go",
				Status:     "modified",
				Additions:  5,
				Deletions:  2,
				Summary:    "Updated logic",
				KeyChanges: []string{},
			},
		},
		TotalAdditions: 5,
		TotalDeletions: 2,
		FilesModified:  1,
		GeneratedAt:    time.Now(),
	}

	result := FormatAsMarkdown(report)

	// Should not contain Duration line when duration is zero
	if strings.Contains(result, "Duration") {
		t.Error("Should not include duration when it's zero")
	}
}

func TestFormatForCommit_DeletedFile(t *testing.T) {
	report := &WorkspaceDiffReport{
		Files: []FileDiffReport{
			{
				Path:       "deprecated.go",
				Status:     "deleted",
				Additions:  0,
				Deletions:  50,
				Summary:    "Removed deprecated code",
				KeyChanges: []string{},
			},
		},
		TotalAdditions: 0,
		TotalDeletions: 50,
		FilesDeleted:   1,
		GeneratedAt:    time.Now(),
	}

	result := FormatForCommit(report)

	if !strings.Contains(result, "D deprecated.go") {
		t.Error("Missing deleted file indicator")
	}
	if !strings.Contains(result, "1 file(s) (+0/-50)") {
		t.Errorf("Missing summary, got:\n%s", result)
	}
}

func TestGenerateReport_KeyChangesWithInterface(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")

	diff := `+type Storage interface {
+	Get(key string) ([]byte, error)
+	Put(key string, value []byte) error
+}
`

	report := dr.GenerateReport(map[string]string{
		"storage.go": diff,
	})

	if len(report.Files) != 1 {
		t.Fatalf("Files count = %d, want 1", len(report.Files))
	}

	f := report.Files[0]
	foundInterface := false
	for _, kc := range f.KeyChanges {
		if strings.Contains(kc, "Storage") && strings.Contains(kc, "interface") {
			foundInterface = true
		}
	}
	if !foundInterface {
		t.Errorf("Expected interface key change, got: %v", f.KeyChanges)
	}
}

func TestGenerateReport_ConcurrentAccess(t *testing.T) {
	dr := NewDiffReporter("/tmp/project")

	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(n int) {
			diff := "+func Concurrent() {}\n"
			report := dr.GenerateReport(map[string]string{
				"file.go": diff,
			})
			if report == nil {
				t.Errorf("Got nil report from goroutine %d", n)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}
}
