package engine

import (
	"strings"
	"testing"
)

func TestDetect_GoErrorWrapping(t *testing.T) {
	detector := NewActionDetector()
	content := `package main

func handle() error {
	err := doSomething()
	if err != nil { return err }
	return nil
}
`
	actions := detector.Detect("main.go", content)

	found := false
	for _, a := range actions {
		if a.ID == "go-err-wrap" {
			found = true
			if a.Category != "refactor" {
				t.Errorf("expected category 'refactor', got %q", a.Category)
			}
			if a.Fix == "" {
				t.Error("expected a fix suggestion for error wrapping")
			}
			if !strings.Contains(a.Fix, "%w") {
				t.Errorf("expected fix to contain %%w, got %q", a.Fix)
			}
			break
		}
	}
	if !found {
		t.Error("expected go-err-wrap action to be detected")
	}
}

func TestDetect_GoDeprecatedAPI(t *testing.T) {
	detector := NewActionDetector()
	content := `package main

import "io/ioutil"

func read() {
	data, _ := ioutil.ReadFile("test.txt")
	_ = data
}
`
	actions := detector.Detect("main.go", content)

	found := false
	for _, a := range actions {
		if a.ID == "go-ioutil-readfile" {
			found = true
			if !strings.Contains(a.Fix, "os.ReadFile") {
				t.Errorf("expected fix to suggest os.ReadFile, got %q", a.Fix)
			}
			if a.Priority != 3 {
				t.Errorf("expected priority 3, got %d", a.Priority)
			}
			break
		}
	}
	if !found {
		t.Error("expected go-ioutil-readfile action to be detected")
	}
}

func TestDetect_GoPerformancePatterns(t *testing.T) {
	detector := NewActionDetector()

	t.Run("string concatenation in loop", func(t *testing.T) {
		content := `package main

func build() string {
	result := ""
	for i := 0; i < 100; i++ {
		result += "item"
	}
	return result
}
`
		actions := detector.Detect("main.go", content)
		found := false
		for _, a := range actions {
			if a.ID == "go-string-concat-loop" {
				found = true
				if a.Category != "performance" {
					t.Errorf("expected category 'performance', got %q", a.Category)
				}
				break
			}
		}
		if !found {
			t.Error("expected go-string-concat-loop action to be detected")
		}
	})

	t.Run("interface{} usage", func(t *testing.T) {
		content := `package main

func process(data interface{}) {
	_ = data
}
`
		actions := detector.Detect("main.go", content)
		found := false
		for _, a := range actions {
			if a.ID == "go-interface-any" {
				found = true
				if !strings.Contains(a.Fix, "any") {
					t.Errorf("expected fix to suggest 'any', got %q", a.Fix)
				}
				break
			}
		}
		if !found {
			t.Error("expected go-interface-any action to be detected")
		}
	})
}

func TestDetect_PythonBareExcept(t *testing.T) {
	detector := NewActionDetector()
	content := `def risky():
    try:
        do_stuff()
    except:
        pass
`
	actions := detector.Detect("handler.py", content)

	found := false
	for _, a := range actions {
		if a.ID == "py-bare-except" {
			found = true
			if a.Category != "fix" {
				t.Errorf("expected category 'fix', got %q", a.Category)
			}
			if a.Priority != 1 {
				t.Errorf("expected priority 1, got %d", a.Priority)
			}
			if !strings.Contains(a.Fix, "except Exception:") {
				t.Errorf("expected fix to suggest 'except Exception:', got %q", a.Fix)
			}
			break
		}
	}
	if !found {
		t.Error("expected py-bare-except action to be detected")
	}
}

func TestDetect_TypeScriptAnyType(t *testing.T) {
	detector := NewActionDetector()
	content := `function handle(req: any, res: any): void {
  const data: any = req.body;
  res.json(data);
}
`
	actions := detector.Detect("handler.ts", content)

	count := 0
	for _, a := range actions {
		if a.ID == "ts-any-type" {
			count++
			if a.Category != "refactor" {
				t.Errorf("expected category 'refactor', got %q", a.Category)
			}
		}
	}
	if count == 0 {
		t.Error("expected ts-any-type actions to be detected")
	}
	if count < 2 {
		t.Errorf("expected at least 2 ts-any-type detections, got %d", count)
	}
}

func TestDetect_UniversalTODO(t *testing.T) {
	detector := NewActionDetector()
	content := `package main

// TODO: fix this later
func broken() {
	// FIXME: handle edge case
	// HACK: workaround for now
}
`
	actions := detector.Detect("main.go", content)

	count := 0
	for _, a := range actions {
		if a.ID == "todo-comment" {
			count++
			if a.Priority != 5 {
				t.Errorf("expected priority 5, got %d", a.Priority)
			}
		}
	}
	if count < 3 {
		t.Errorf("expected at least 3 TODO/FIXME/HACK detections, got %d", count)
	}
}

func TestFormatSuggestions(t *testing.T) {
	actions := []CodeAction{
		{
			ID:       "go-err-wrap",
			Title:    "Wrap error with context",
			Category: "refactor",
			File:     "src/handler.go",
			Line:     15,
			Priority: 3,
			Fix:      `return fmt.Errorf("handle request: %w", err)`,
		},
		{
			ID:       "go-append-loop",
			Title:    "Pre-allocate slice",
			Category: "performance",
			File:     "src/handler.go",
			Line:     42,
			Priority: 3,
			Fix:      "make([]User, 0, len(input))",
		},
		{
			ID:       "hardcoded-credential",
			Title:    "Possible hardcoded credential",
			Category: "security",
			File:     "src/handler.go",
			Line:     67,
			Priority: 1,
		},
	}

	output := FormatSuggestions(actions, 5)

	if !strings.Contains(output, "Suggestions for src/handler.go:") {
		t.Error("expected header with file path")
	}
	if !strings.Contains(output, "[refactor]") {
		t.Error("expected [refactor] category tag")
	}
	if !strings.Contains(output, "[performance]") {
		t.Error("expected [performance] category tag")
	}
	if !strings.Contains(output, "[security]") {
		t.Error("expected [security] category tag")
	}
	if !strings.Contains(output, "L15") {
		t.Error("expected line number L15")
	}
	if !strings.Contains(output, "L42") {
		t.Error("expected line number L42")
	}
	if !strings.Contains(output, `fmt.Errorf`) {
		t.Error("expected fix text in output")
	}
	// security action has no fix so should not have " - " line
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Possible hardcoded credential") {
			// Next line should not be a fix line for this action
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "- ") {
				// Check it's not an empty fix
				fixLine := strings.TrimSpace(lines[i+1])
				if fixLine == "- " || fixLine == "-" {
					t.Error("should not display empty fix line")
				}
			}
		}
	}
}

func TestFormatSuggestions_MaxDisplay(t *testing.T) {
	actions := make([]CodeAction, 10)
	for i := range actions {
		actions[i] = CodeAction{
			ID:       "test",
			Title:    "Test action",
			Category: "style",
			File:     "test.go",
			Line:     i + 1,
			Priority: 5,
		}
	}

	output := FormatSuggestions(actions, 3)
	if !strings.Contains(output, "... and 7 more suggestions") {
		t.Errorf("expected truncation message, got:\n%s", output)
	}
}

func TestApplyFix(t *testing.T) {
	content := `package main

import "io/ioutil"

func main() {
	data, _ := ioutil.ReadFile("test.txt")
	_ = data
}
`
	action := CodeAction{
		ID:   "go-ioutil-readfile",
		Title: "Replace deprecated ioutil.ReadFile",
		File: "main.go",
		Line: 6,
		Fix:  "os.ReadFile",
	}

	result, err := applyFixHelper(action, content)
	if err != nil {
		t.Fatalf("ApplyFix failed: %v", err)
	}
	if !strings.Contains(result, "os.ReadFile") {
		t.Error("expected result to contain os.ReadFile")
	}
	if strings.Contains(result, "ioutil.ReadFile") {
		t.Error("expected ioutil.ReadFile to be replaced")
	}
}

func applyFixHelper(action CodeAction, content string) (string, error) {
	return ApplyFix(action, content)
}

func TestApplyFix_InvalidLine(t *testing.T) {
	content := "line1\nline2\n"

	action := CodeAction{
		ID:   "test",
		Line: 100,
		Fix:  "replacement",
	}

	_, err := ApplyFix(action, content)
	if err == nil {
		t.Error("expected error for invalid line number")
	}
}

func TestApplyFix_NoFix(t *testing.T) {
	content := "line1\nline2\n"

	action := CodeAction{
		ID:   "test",
		Line: 1,
		Fix:  "",
	}

	_, err := ApplyFix(action, content)
	if err == nil {
		t.Error("expected error when no fix is available")
	}
}

func TestDetect_AntipatternSuppression(t *testing.T) {
	detector := NewActionDetector()

	t.Run("proper except is not flagged", func(t *testing.T) {
		content := `def safe():
    try:
        do_stuff()
    except ValueError:
        pass
`
		actions := detector.Detect("handler.py", content)
		for _, a := range actions {
			if a.ID == "py-bare-except" {
				t.Error("py-bare-except should not fire when except has a type")
			}
		}
	})

	t.Run("strict equality not flagged", func(t *testing.T) {
		content := `if (value === null) { return; }
`
		actions := detector.Detect("app.ts", content)
		for _, a := range actions {
			if a.ID == "ts-loose-equality-null" {
				t.Error("ts-loose-equality-null should not fire for === null")
			}
		}
	})
}

func TestDetect_PriorityOrdering(t *testing.T) {
	detector := NewActionDetector()
	content := `package main

// TODO: fix this
func process(data interface{}) {
	password := "secret123"
	if err != nil { return err }
}
`
	actions := detector.Detect("main.go", content)

	if len(actions) < 2 {
		t.Fatalf("expected at least 2 actions, got %d", len(actions))
	}

	// Verify sorted by priority (ascending)
	for i := 1; i < len(actions); i++ {
		if actions[i].Priority < actions[i-1].Priority {
			t.Errorf("actions not sorted by priority: action[%d].Priority=%d < action[%d].Priority=%d",
				i, actions[i].Priority, i-1, actions[i-1].Priority)
		}
	}
}

func TestDetectForDiff(t *testing.T) {
	detector := NewActionDetector()
	diff := `diff --git a/handler.py b/handler.py
index abc123..def456 100644
--- a/handler.py
+++ b/handler.py
@@ -10,6 +10,10 @@ def existing():
     pass

+def risky():
+    try:
+        do_stuff()
+    except:
+        pass
`
	actions := detector.DetectForDiff(diff)

	found := false
	for _, a := range actions {
		if a.ID == "py-bare-except" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected py-bare-except to be detected in diff added lines")
	}
}

func TestDetect_SecurityPatterns(t *testing.T) {
	detector := NewActionDetector()
	content := `package main

const dbPassword = "supersecret123"

func query(userInput string) {
	q := "SELECT * FROM users WHERE name=" + userInput
	_ = q
}
`
	actions := detector.Detect("main.go", content)

	var foundCredential, foundSQL bool
	for _, a := range actions {
		if a.ID == "hardcoded-credential" {
			foundCredential = true
			if a.Priority != 1 {
				t.Errorf("expected priority 1 for security issue, got %d", a.Priority)
			}
			if a.Category != "security" {
				t.Errorf("expected category 'security', got %q", a.Category)
			}
		}
		if a.ID == "sql-injection" {
			foundSQL = true
		}
	}
	if !foundCredential {
		t.Error("expected hardcoded-credential to be detected")
	}
	if !foundSQL {
		t.Error("expected sql-injection to be detected")
	}
}

func TestDetect_LanguageFiltering(t *testing.T) {
	detector := NewActionDetector()

	// Python rules should not fire on Go files
	content := `except:
    pass
`
	actions := detector.Detect("main.go", content)
	for _, a := range actions {
		if a.ID == "py-bare-except" {
			t.Error("python rule should not fire on .go file")
		}
	}
}

func TestDetect_DeepNesting(t *testing.T) {
	detector := NewActionDetector()
	content := `package main

func deep() {
	if true {
		if true {
			if true {
				if true {
					if true {
						doSomething()
					}
				}
			}
		}
	}
}
`
	actions := detector.Detect("main.go", content)
	found := false
	for _, a := range actions {
		if a.ID == "deep-nesting" {
			found = true
			if a.Category != "refactor" {
				t.Errorf("expected category 'refactor', got %q", a.Category)
			}
			break
		}
	}
	if !found {
		t.Error("expected deep-nesting action to be detected")
	}
}

func TestNewActionDetector_HasMinimumRules(t *testing.T) {
	detector := NewActionDetector()
	if len(detector.Rules) < 25 {
		t.Errorf("expected at least 25 built-in rules, got %d", len(detector.Rules))
	}
}

func TestDetect_EmptyContent(t *testing.T) {
	detector := NewActionDetector()
	actions := detector.Detect("empty.go", "")
	if len(actions) != 0 {
		t.Errorf("expected no actions for empty content, got %d", len(actions))
	}
}

func TestFormatSuggestions_Empty(t *testing.T) {
	output := FormatSuggestions(nil, 5)
	if output != "" {
		t.Errorf("expected empty output for nil actions, got %q", output)
	}
}
