package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewPatternLibrary(t *testing.T) {
	pl := NewPatternLibrary("/tmp/patterns")
	if pl == nil {
		t.Fatal("expected non-nil PatternLibrary")
	}
	if pl.Dir != "/tmp/patterns" {
		t.Errorf("expected Dir=/tmp/patterns, got %s", pl.Dir)
	}
	if pl.Patterns == nil {
		t.Error("expected non-nil Patterns map")
	}
	if len(pl.Patterns) != 0 {
		t.Errorf("expected empty Patterns map, got %d entries", len(pl.Patterns))
	}
}

func TestLoadBuiltins(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	expectedPatterns := []string{
		"summarize", "explain_code", "find_bugs", "improve_code",
		"write_tests", "extract_todos", "security_review", "api_docs",
		"commit_message", "review_pr", "debug_error", "refactor_plan",
		"architecture_doc", "performance_review", "accessibility_check",
	}

	if len(pl.Patterns) < 15 {
		t.Errorf("expected at least 15 built-in patterns, got %d", len(pl.Patterns))
	}

	for _, name := range expectedPatterns {
		p := pl.Get(name)
		if p == nil {
			t.Errorf("expected built-in pattern %q to exist", name)
			continue
		}
		if p.Name != name {
			t.Errorf("pattern %q: Name mismatch, got %q", name, p.Name)
		}
		if p.Description == "" {
			t.Errorf("pattern %q: empty Description", name)
		}
		if p.SystemPrompt == "" {
			t.Errorf("pattern %q: empty SystemPrompt", name)
		}
		if p.UserTemplate == "" {
			t.Errorf("pattern %q: empty UserTemplate", name)
		}
		if !strings.Contains(p.UserTemplate, "{{INPUT}}") {
			t.Errorf("pattern %q: UserTemplate missing {{INPUT}} placeholder", name)
		}
		if p.Version == "" {
			t.Errorf("pattern %q: empty Version", name)
		}
		if p.Author == "" {
			t.Errorf("pattern %q: empty Author", name)
		}
		if len(p.Tags) == 0 {
			t.Errorf("pattern %q: empty Tags", name)
		}
	}
}

func TestPatternsGet(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	// Existing pattern
	p := pl.Get("summarize")
	if p == nil {
		t.Fatal("expected to get 'summarize' pattern")
	}
	if p.Name != "summarize" {
		t.Errorf("expected Name=summarize, got %s", p.Name)
	}

	// Non-existing pattern
	p = pl.Get("nonexistent")
	if p != nil {
		t.Error("expected nil for non-existent pattern")
	}
}

func TestApply(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	input := "func main() { fmt.Println(\"hello\") }"

	sys, user, err := pl.Apply("explain_code", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sys == "" {
		t.Error("expected non-empty system prompt")
	}
	if !strings.Contains(user, input) {
		t.Error("expected user prompt to contain input")
	}
	if strings.Contains(user, "{{INPUT}}") {
		t.Error("expected {{INPUT}} to be replaced")
	}

	// Non-existent pattern
	_, _, err = pl.Apply("nonexistent", input)
	if err == nil {
		t.Error("expected error for non-existent pattern")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestPatternsSearch(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	// Search by name substring
	results := pl.Search("bug")
	found := false
	for _, r := range results {
		if r.Name == "find_bugs" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'find_bugs' when searching for 'bug'")
	}

	// Search by tag
	results = pl.Search("security")
	found = false
	for _, r := range results {
		if r.Name == "security_review" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'security_review' when searching for 'security'")
	}

	// Search by description
	results = pl.Search("junior")
	found = false
	for _, r := range results {
		if r.Name == "explain_code" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'explain_code' when searching for 'junior'")
	}

	// Case insensitive
	results = pl.Search("BUG")
	found = false
	for _, r := range results {
		if r.Name == "find_bugs" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected case-insensitive search to find 'find_bugs'")
	}

	// No results
	results = pl.Search("zzzyyyxxx")
	if len(results) != 0 {
		t.Errorf("expected no results for gibberish query, got %d", len(results))
	}
}

func TestRegisterAndRemove(t *testing.T) {
	pl := NewPatternLibrary("")

	p := &PromptPattern{
		Name:         "custom_pattern",
		Description:  "A custom pattern",
		SystemPrompt: "You are helpful.",
		UserTemplate: "Do this: {{INPUT}}",
		OutputFormat: "text",
		Tags:         []string{"custom"},
		Version:      "0.1.0",
		Author:       "test",
	}

	pl.Register(p)

	got := pl.Get("custom_pattern")
	if got == nil {
		t.Fatal("expected registered pattern to exist")
	}
	if got.Description != "A custom pattern" {
		t.Errorf("unexpected description: %s", got.Description)
	}

	// Overwrite
	p2 := &PromptPattern{
		Name:         "custom_pattern",
		Description:  "Updated description",
		SystemPrompt: "You are very helpful.",
		UserTemplate: "Please: {{INPUT}}",
		Version:      "0.2.0",
		Author:       "test",
	}
	pl.Register(p2)
	got = pl.Get("custom_pattern")
	if got.Description != "Updated description" {
		t.Error("expected pattern to be overwritten")
	}

	// Remove
	pl.Remove("custom_pattern")
	got = pl.Get("custom_pattern")
	if got != nil {
		t.Error("expected pattern to be removed")
	}

	// Remove non-existent (should not panic)
	pl.Remove("nonexistent")
}

func TestChain(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	input := "func add(a, b int) int { return a + b }"
	chain := []string{"explain_code", "summarize"}

	results := pl.Chain(chain, input)
	if len(results) != 2 {
		t.Fatalf("expected 2 results from chain, got %d", len(results))
	}

	// First result should contain original input
	if !strings.Contains(results[0], input) {
		t.Error("first chain result should contain original input")
	}

	// Second result should contain first result (chained)
	if !strings.Contains(results[1], results[0]) {
		t.Error("second chain result should contain first result as input")
	}

	// Chain with non-existent pattern
	chain = []string{"explain_code", "nonexistent", "summarize"}
	results = pl.Chain(chain, input)
	if len(results) != 2 {
		t.Fatalf("expected 2 results (stop at error), got %d", len(results))
	}
	if !strings.Contains(results[1], "[error:") {
		t.Error("expected error marker in results")
	}
}

func TestLoadFromDir(t *testing.T) {
	// Create temp directory with pattern files
	dir := t.TempDir()

	// Valid pattern file
	content := `---
name: test_pattern
description: A test pattern
system_prompt: You are a test helper.
output_format: text
tags: [test, example]
version: 1.0.0
author: tester
---
Test this input:

{{INPUT}}
`
	err := os.WriteFile(filepath.Join(dir, "test_pattern.md"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Another valid pattern with quoted tags
	content2 := `---
name: another_pattern
description: Another test
system_prompt: Be helpful.
output_format: markdown
tags: "code, review"
version: 0.1.0
author: someone
---
Review: {{INPUT}}
`
	err = os.WriteFile(filepath.Join(dir, "another.md"), []byte(content2), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Non-markdown file (should be ignored)
	err = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a pattern"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	// Malformed file (no frontmatter)
	err = os.WriteFile(filepath.Join(dir, "bad.md"), []byte("no frontmatter here"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	pl := NewPatternLibrary(dir)
	err = pl.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check loaded patterns
	p := pl.Get("test_pattern")
	if p == nil {
		t.Fatal("expected test_pattern to be loaded")
	}
	if p.Description != "A test pattern" {
		t.Errorf("unexpected description: %s", p.Description)
	}
	if p.SystemPrompt != "You are a test helper." {
		t.Errorf("unexpected system prompt: %s", p.SystemPrompt)
	}
	if !strings.Contains(p.UserTemplate, "{{INPUT}}") {
		t.Error("expected UserTemplate to contain {{INPUT}}")
	}
	if len(p.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(p.Tags))
	}

	p2 := pl.Get("another_pattern")
	if p2 == nil {
		t.Fatal("expected another_pattern to be loaded")
	}

	// Non-existent directory
	pl2 := NewPatternLibrary("/tmp/nonexistent_patterns_dir_xyz")
	err = pl2.LoadFromDir("/tmp/nonexistent_patterns_dir_xyz")
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestLoadFromDir_NameFromFilename(t *testing.T) {
	dir := t.TempDir()

	// Pattern file without name in frontmatter
	content := `---
description: Pattern without explicit name
system_prompt: Be helpful.
output_format: text
version: 1.0.0
author: tester
---
Do something: {{INPUT}}
`
	err := os.WriteFile(filepath.Join(dir, "my_custom_pattern.md"), []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	pl := NewPatternLibrary(dir)
	err = pl.LoadFromDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	p := pl.Get("my_custom_pattern")
	if p == nil {
		t.Fatal("expected pattern to use filename as name")
	}
}

func TestFormatPattern(t *testing.T) {
	p := &PromptPattern{
		Name:         "test",
		Description:  "Test pattern",
		SystemPrompt: "You are a test.",
		UserTemplate: "Test: {{INPUT}}",
		OutputFormat: "text",
		Tags:         []string{"test", "example"},
		Version:      "1.0.0",
		Author:       "tester",
	}

	formatted := FormatPattern(p)
	if !strings.Contains(formatted, "test") {
		t.Error("expected formatted output to contain pattern name")
	}
	if !strings.Contains(formatted, "Test pattern") {
		t.Error("expected formatted output to contain description")
	}
	if !strings.Contains(formatted, "1.0.0") {
		t.Error("expected formatted output to contain version")
	}
	if !strings.Contains(formatted, "tester") {
		t.Error("expected formatted output to contain author")
	}
	if !strings.Contains(formatted, "test, example") {
		t.Error("expected formatted output to contain tags")
	}
}

func TestFormatPattern_Truncation(t *testing.T) {
	longPrompt := strings.Repeat("a", 200)
	p := &PromptPattern{
		Name:         "long",
		Description:  "Long pattern",
		SystemPrompt: longPrompt,
		UserTemplate: longPrompt,
		OutputFormat: "text",
		Version:      "1.0.0",
		Author:       "tester",
	}

	formatted := FormatPattern(p)
	if strings.Contains(formatted, longPrompt) {
		t.Error("expected long strings to be truncated")
	}
	if !strings.Contains(formatted, "...") {
		t.Error("expected truncation marker '...'")
	}
}

func TestListByTag(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	// Search for code tag
	results := pl.ListByTag("code")
	if len(results) == 0 {
		t.Error("expected results for 'code' tag")
	}
	for _, r := range results {
		hasTag := false
		for _, tag := range r.Tags {
			if strings.ToLower(tag) == "code" {
				hasTag = true
				break
			}
		}
		if !hasTag {
			t.Errorf("pattern %q in results but doesn't have 'code' tag", r.Name)
		}
	}

	// Case insensitive tag search
	results2 := pl.ListByTag("CODE")
	if len(results2) != len(results) {
		t.Error("expected case-insensitive tag matching")
	}

	// Non-existent tag
	results = pl.ListByTag("zzzyyyxxx")
	if len(results) != 0 {
		t.Errorf("expected no results for non-existent tag, got %d", len(results))
	}

	// Specific tag
	results = pl.ListByTag("a11y")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'a11y' tag, got %d", len(results))
	}
	if results[0].Name != "accessibility_check" {
		t.Errorf("expected accessibility_check, got %s", results[0].Name)
	}
}

func TestPatternsConcurrentAccess(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	var wg sync.WaitGroup
	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pl.Get("summarize")
			pl.Search("code")
			pl.ListByTag("review")
		}()
	}

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			p := &PromptPattern{
				Name:         fmt.Sprintf("concurrent_%d", n),
				Description:  "Concurrent test",
				UserTemplate: "{{INPUT}}",
			}
			pl.Register(p)
		}(i)
	}

	wg.Wait()

	// Verify no panic and patterns exist
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("concurrent_%d", i)
		if pl.Get(name) == nil {
			t.Errorf("expected concurrent pattern %s to exist", name)
		}
	}
}

func TestApplyMultipleInputPlaceholders(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.Register(&PromptPattern{
		Name:         "multi",
		Description:  "Multiple placeholders",
		SystemPrompt: "System",
		UserTemplate: "First: {{INPUT}}\nSecond: {{INPUT}}",
	})

	_, user, err := pl.Apply("multi", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if user != "First: hello\nSecond: hello" {
		t.Errorf("unexpected result: %s", user)
	}
}

func TestChainEmpty(t *testing.T) {
	pl := NewPatternLibrary("")
	pl.LoadBuiltins()

	// Empty chain
	results := pl.Chain([]string{}, "input")
	if len(results) != 0 {
		t.Errorf("expected empty results for empty chain, got %d", len(results))
	}

	// Single pattern chain
	results = pl.Chain([]string{"summarize"}, "some text")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !strings.Contains(results[0], "some text") {
		t.Error("expected result to contain input")
	}
}
