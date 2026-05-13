package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewConflictResolver(t *testing.T) {
	cr := NewConflictResolver()
	if cr == nil {
		t.Fatal("NewConflictResolver returned nil")
	}
	if cr.Strategy != "smart" {
		t.Errorf("expected strategy 'smart', got %q", cr.Strategy)
	}
}

func TestParseConflicts_Basic(t *testing.T) {
	content := `package main

import "fmt"

<<<<<<< HEAD
func hello() {
	fmt.Println("hello from ours")
}
=======
func hello() {
	fmt.Println("hello from theirs")
}
>>>>>>> feature-branch

func main() {
	hello()
}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	if len(cf.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(cf.Conflicts))
	}

	c := cf.Conflicts[0]
	if !strings.Contains(c.OursContent, "hello from ours") {
		t.Errorf("ours content missing expected text: %q", c.OursContent)
	}
	if !strings.Contains(c.TheirsContent, "hello from theirs") {
		t.Errorf("theirs content missing expected text: %q", c.TheirsContent)
	}
	if c.StartLine != 5 {
		t.Errorf("expected start line 5, got %d", c.StartLine)
	}
}

func TestParseConflicts_ThreeWay(t *testing.T) {
	content := `line1
<<<<<<< HEAD
ours line
||||||| merged common ancestors
base line
=======
theirs line
>>>>>>> branch
line2
`
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	if len(cf.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(cf.Conflicts))
	}

	c := cf.Conflicts[0]
	if c.OursContent != "ours line" {
		t.Errorf("expected ours 'ours line', got %q", c.OursContent)
	}
	if c.BaseContent != "base line" {
		t.Errorf("expected base 'base line', got %q", c.BaseContent)
	}
	if c.TheirsContent != "theirs line" {
		t.Errorf("expected theirs 'theirs line', got %q", c.TheirsContent)
	}
}

func TestParseConflicts_Multiple(t *testing.T) {
	content := `start
<<<<<<< HEAD
conflict1 ours
=======
conflict1 theirs
>>>>>>> branch
middle
<<<<<<< HEAD
conflict2 ours
=======
conflict2 theirs
>>>>>>> branch
end
`
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	if len(cf.Conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(cf.Conflicts))
	}
}

func TestParseConflicts_NoConflicts(t *testing.T) {
	content := "just a normal file\nwith no conflicts\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	_, err := cr.ParseConflicts(path)
	if err == nil {
		t.Error("expected error for file with no conflicts")
	}
}

func TestResolveTrivial_Identical(t *testing.T) {
	resolution, ok := ResolveTrivial("same content", "same content")
	if !ok {
		t.Fatal("expected trivial resolution for identical content")
	}
	if resolution != "same content" {
		t.Errorf("expected 'same content', got %q", resolution)
	}
}

func TestResolveTrivial_OursEmpty(t *testing.T) {
	resolution, ok := ResolveTrivial("", "theirs content")
	if !ok {
		t.Fatal("expected trivial resolution when ours is empty")
	}
	if resolution != "theirs content" {
		t.Errorf("expected 'theirs content', got %q", resolution)
	}
}

func TestResolveTrivial_TheirsEmpty(t *testing.T) {
	resolution, ok := ResolveTrivial("ours content", "   ")
	if !ok {
		t.Fatal("expected trivial resolution when theirs is whitespace-only")
	}
	if resolution != "ours content" {
		t.Errorf("expected 'ours content', got %q", resolution)
	}
}

func TestResolveTrivial_WhitespaceOnly(t *testing.T) {
	resolution, ok := ResolveTrivial("  hello  ", "hello")
	if !ok {
		t.Fatal("expected trivial resolution for whitespace-only difference")
	}
	if resolution != "  hello  " {
		t.Errorf("expected '  hello  ', got %q", resolution)
	}
}

func TestResolveTrivial_NotResolvable(t *testing.T) {
	_, ok := ResolveTrivial("different content", "very different content")
	if ok {
		t.Error("expected non-trivial conflict to not be resolved")
	}
}

func TestResolveImports(t *testing.T) {
	ours := `"fmt"
"os"
"strings"`
	theirs := `"fmt"
"io"
"strings"
"sync"`

	result := ResolveImports(ours, theirs)

	// Should contain all unique imports, sorted
	if !strings.Contains(result, `"fmt"`) {
		t.Error("missing fmt")
	}
	if !strings.Contains(result, `"io"`) {
		t.Error("missing io")
	}
	if !strings.Contains(result, `"os"`) {
		t.Error("missing os")
	}
	if !strings.Contains(result, `"strings"`) {
		t.Error("missing strings")
	}
	if !strings.Contains(result, `"sync"`) {
		t.Error("missing sync")
	}

	// Verify sorted
	lines := strings.Split(result, "\n")
	for i := 0; i < len(lines)-1; i++ {
		if lines[i] > lines[i+1] {
			t.Errorf("imports not sorted: %q > %q", lines[i], lines[i+1])
		}
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, line := range lines {
		if seen[line] {
			t.Errorf("duplicate import: %q", line)
		}
		seen[line] = true
	}
}

func TestResolveAdditive(t *testing.T) {
	base := "line1\nline2\nline3"
	ours := "line1\nline2\nline3\nours_added"
	theirs := "line1\nline2\nline3\ntheirs_added"

	result := ResolveAdditive(ours, theirs, base)
	if result == "" {
		t.Fatal("expected additive resolution to succeed")
	}

	if !strings.Contains(result, "ours_added") {
		t.Error("result missing ours_added")
	}
	if !strings.Contains(result, "theirs_added") {
		t.Error("result missing theirs_added")
	}
	if !strings.Contains(result, "line1") {
		t.Error("result missing base content")
	}
}

func TestResolveAdditive_EmptyBase(t *testing.T) {
	result := ResolveAdditive("ours", "theirs", "")
	if result != "" {
		t.Errorf("expected empty result for empty base, got %q", result)
	}
}

func TestAutoResolve_TrivialIdentical(t *testing.T) {
	content := `before
<<<<<<< HEAD
same content
=======
same content
>>>>>>> branch
after
`
	dir := t.TempDir()
	path := filepath.Join(dir, "trivial.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	resolved, err := cr.AutoResolve(cf)
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}

	if strings.Contains(resolved, "<<<<<<<") {
		t.Error("resolved content still contains conflict markers")
	}
	if !strings.Contains(resolved, "same content") {
		t.Error("resolved content missing the deduplicated content")
	}
	if !cf.Conflicts[0].Resolved {
		t.Error("conflict should be marked as resolved")
	}
}

func TestAutoResolve_StrategyOurs(t *testing.T) {
	content := `before
<<<<<<< HEAD
ours version
=======
theirs version
>>>>>>> branch
after
`
	dir := t.TempDir()
	path := filepath.Join(dir, "ours.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cr.Strategy = "ours"
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	resolved, err := cr.AutoResolve(cf)
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}

	if !strings.Contains(resolved, "ours version") {
		t.Error("resolved content should contain ours version")
	}
	if strings.Contains(resolved, "theirs version") {
		t.Error("resolved content should not contain theirs version")
	}
}

func TestAutoResolve_StrategyTheirs(t *testing.T) {
	content := `before
<<<<<<< HEAD
ours version
=======
theirs version
>>>>>>> branch
after
`
	dir := t.TempDir()
	path := filepath.Join(dir, "theirs.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cr.Strategy = "theirs"
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	resolved, err := cr.AutoResolve(cf)
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}

	if strings.Contains(resolved, "ours version") {
		t.Error("resolved content should not contain ours version")
	}
	if !strings.Contains(resolved, "theirs version") {
		t.Error("resolved content should contain theirs version")
	}
}

func TestAutoResolve_ImportMerge(t *testing.T) {
	content := `package main

<<<<<<< HEAD
import "fmt"
import "os"
import "strings"
=======
import "fmt"
import "io"
import "strings"
import "sync"
>>>>>>> branch

func main() {}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "imports.go")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	resolved, err := cr.AutoResolve(cf)
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}

	if strings.Contains(resolved, "<<<<<<<") {
		t.Error("resolved content still contains conflict markers")
	}
	// All imports should be present
	for _, imp := range []string{"fmt", "io", "os", "strings", "sync"} {
		if !strings.Contains(resolved, imp) {
			t.Errorf("resolved content missing import %q", imp)
		}
	}
}

func TestAutoResolve_OneEmpty(t *testing.T) {
	content := `before
<<<<<<< HEAD
=======
new content added
>>>>>>> branch
after
`
	dir := t.TempDir()
	path := filepath.Join(dir, "empty_ours.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	resolved, err := cr.AutoResolve(cf)
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}

	if !strings.Contains(resolved, "new content added") {
		t.Error("should keep theirs when ours is empty")
	}
	if strings.Contains(resolved, "<<<<<<<") {
		t.Error("should not contain conflict markers")
	}
}

func TestApplyResolution(t *testing.T) {
	content := `before
<<<<<<< HEAD
same
=======
same
>>>>>>> branch
after
`
	dir := t.TempDir()
	path := filepath.Join(dir, "apply.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	_, err = cr.AutoResolve(cf)
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}

	err = cr.ApplyResolution(cf)
	if err != nil {
		t.Fatalf("ApplyResolution failed: %v", err)
	}

	// Read back and verify
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := string(data)
	if strings.Contains(result, "<<<<<<<") {
		t.Error("written file still contains conflict markers")
	}
	if !strings.Contains(result, "same") {
		t.Error("written file missing resolved content")
	}
}

func TestFormatConflicts(t *testing.T) {
	cf := &ConflictFile{
		Path: "src/auth.go",
		Conflicts: []Conflict{
			{
				StartLine:    15,
				EndLine:      22,
				OursContent:  `import "fmt"`,
				TheirsContent: `import "os"`,
				Resolved:     true,
				Resolution:   `import "fmt"\nimport "os"`,
			},
			{
				StartLine:    45,
				EndLine:      60,
				OursContent:  "func auth() { return true }",
				TheirsContent: "func auth() { return false }",
				Resolved:     false,
			},
		},
	}

	output := FormatConflicts(cf)

	if !strings.Contains(output, "src/auth.go") {
		t.Error("output should contain file path")
	}
	if !strings.Contains(output, "Conflict 1") {
		t.Error("output should contain conflict numbering")
	}
	if !strings.Contains(output, "L15-L22") {
		t.Error("output should contain line numbers")
	}
	if !strings.Contains(output, "AUTO") {
		t.Error("output should show AUTO for resolved conflicts")
	}
	if !strings.Contains(output, "MANUAL NEEDED") {
		t.Error("output should show MANUAL NEEDED for unresolved conflicts")
	}
	if !strings.Contains(output, "1 auto-resolved") {
		t.Error("output should contain summary with auto-resolved count")
	}
	if !strings.Contains(output, "1 needs review") {
		t.Error("output should contain summary with needs-review count")
	}
}

func TestConflictResolverTool_Interface(t *testing.T) {
	var tool Tool = ConflictResolverTool{}
	if tool.Name() != "ResolveConflicts" {
		t.Errorf("expected name 'ResolveConflicts', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description should not be empty")
	}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("parameters should not be nil")
	}
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("parameters should have properties")
	}
	if _, ok := props["path"]; !ok {
		t.Error("parameters should include 'path'")
	}
	if _, ok := props["strategy"]; !ok {
		t.Error("parameters should include 'strategy'")
	}
	if _, ok := props["dry_run"]; !ok {
		t.Error("parameters should include 'dry_run'")
	}
}

func TestConflictResolverTool_Execute(t *testing.T) {
	content := `line1
<<<<<<< HEAD
same content here
=======
same content here
>>>>>>> branch
line2
`
	dir := t.TempDir()
	path := filepath.Join(dir, "execute.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tool := ConflictResolverTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"path":     path,
		"strategy": "smart",
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(result, "auto-resolved") {
		t.Error("result should contain resolution summary")
	}
	if !strings.Contains(result, "Resolved content written") {
		t.Error("result should confirm file was written")
	}

	// Verify file was actually written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "<<<<<<<") {
		t.Error("file should not contain conflict markers after resolution")
	}
}

func TestConflictResolverTool_DryRun(t *testing.T) {
	content := `before
<<<<<<< HEAD
ours
=======
theirs
>>>>>>> branch
after
`
	dir := t.TempDir()
	path := filepath.Join(dir, "dryrun.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tool := ConflictResolverTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"path":    path,
		"dry_run": true,
	})

	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if strings.Contains(result, "written to") {
		t.Error("dry run should not write file")
	}

	// Verify file was NOT modified
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<<<<<<<") {
		t.Error("file should still contain conflict markers after dry run")
	}
}

func TestConflictResolverTool_MissingPath(t *testing.T) {
	tool := ConflictResolverTool{}
	input, _ := json.Marshal(map[string]interface{}{})

	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestConflictResolverTool_NonexistentFile(t *testing.T) {
	tool := ConflictResolverTool{}
	input, _ := json.Marshal(map[string]interface{}{
		"path": "/nonexistent/path/file.txt",
	})

	_, err := tool.Execute(context.Background(), input)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestAutoResolve_BothAddDifferent(t *testing.T) {
	// When both sides add different non-overlapping lines, combine them
	content := `base
<<<<<<< HEAD
alpha
beta
=======
gamma
delta
>>>>>>> branch
end
`
	dir := t.TempDir()
	path := filepath.Join(dir, "additive.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cr := NewConflictResolver()
	cf, err := cr.ParseConflicts(path)
	if err != nil {
		t.Fatalf("ParseConflicts failed: %v", err)
	}

	resolved, err := cr.AutoResolve(cf)
	if err != nil {
		t.Fatalf("AutoResolve failed: %v", err)
	}

	// Both additions should be present
	if !strings.Contains(resolved, "alpha") {
		t.Error("resolved should contain alpha")
	}
	if !strings.Contains(resolved, "gamma") {
		t.Error("resolved should contain gamma")
	}
}

func TestIsImportBlock(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "go imports",
			content: "import \"fmt\"\nimport \"os\"",
			want:    true,
		},
		{
			name:    "regular code",
			content: "func main() {\n\tfmt.Println(\"hello\")\n}",
			want:    false,
		},
		{
			name:    "empty",
			content: "",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isImportBlock(tt.content)
			if got != tt.want {
				t.Errorf("isImportBlock(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}
