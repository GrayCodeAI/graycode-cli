package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/GrayCodeAI/graycode-cli/internal/spec"
)

type SpecTestFirstTool struct{}

func (SpecTestFirstTool) Name() string { return "SpecTestFirst" }
func (SpecTestFirstTool) Aliases() []string {
	return []string{"spec_test_first", "spec:test_first"}
}

func (SpecTestFirstTool) Description() string {
	return "Reorder tasks to follow test-first development. Places test-writing tasks before implementation tasks. Detects existing test patterns in the codebase and generates matching test tasks."
}

func (SpecTestFirstTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"apply": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, rewrite tasks.md with test-first ordering",
			},
			"framework": map[string]interface{}{
				"type":        "string",
				"description": "Test framework to use: auto, go, jest, vitest, pytest",
				"enum":        []string{"auto", "go", "jest", "vitest", "pytest"},
			},
		},
	}
}

func (SpecTestFirstTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Apply     bool   `json:"apply"`
		Framework string `json:"framework"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	tasksContent := readFileStr(filepath.Join(dir, "tasks.md"))
	if tasksContent == "" {
		return "No tasks.md found. Use Tasks tool first.", nil
	}

	tasks := spec.ParseTasks(tasksContent)
	if len(tasks) == 0 {
		return "No unchecked tasks found.", nil
	}

	if p.Framework == "auto" {
		p.Framework = detectTestFramework(dir)
	}

	ordered := reorderTestFirst(tasks, p.Framework)

	var b strings.Builder
	b.WriteString("## Test-First Task Order\n\n")
	fmt.Fprintf(&b, "**Framework**: %s\n\n", p.Framework)

	b.WriteString("### Reordered Tasks\n\n")
	for i, t := range ordered {
		prefix := ""
		if isTestTask(t) {
			prefix = "[TEST] "
		} else {
			prefix = "[IMPL] "
		}
		fmt.Fprintf(&b, "%d. %s%s\n", i+1, prefix, t.Description)
	}
	b.WriteString("\n")

	if p.Apply {
		newContent := rebuildTasksContent(tasksContent, ordered)
		tasksPath := filepath.Join(dir, "tasks.md")
		if err := os.WriteFile(tasksPath, []byte(newContent), 0o600); err != nil {
			return "", fmt.Errorf("write tasks.md: %w", err)
		}
		b.WriteString("**tasks.md updated with test-first ordering.**\n")
	} else {
		b.WriteString("Call with `apply: true` to rewrite tasks.md.\n")
	}

	return strings.TrimSpace(b.String()), nil
}

func detectTestFramework(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(dir, "jest.config.js")); err == nil {
		return "jest"
	}
	if _, err := os.Stat(filepath.Join(dir, "vitest.config.ts")); err == nil {
		return "vitest"
	}
	if _, err := os.Stat(filepath.Join(dir, "pytest.ini")); err == nil {
		return "pytest"
	}
	return "go"
}

func isTestTask(t spec.Task) bool {
	lower := strings.ToLower(t.Description)
	return strings.Contains(lower, "test") ||
		strings.Contains(lower, "spec") ||
		strings.Contains(lower, "unit test") ||
		strings.Contains(lower, "integration test") ||
		strings.Contains(lower, "e2e test")
}

func reorderTestFirst(tasks []spec.Task, framework string) []spec.Task {
	var tests, impls, others []spec.Task

	for _, t := range tasks {
		if isTestTask(t) {
			tests = append(tests, t)
		} else if needsTest(t, framework) {
			tests = append(tests, spec.Task{
				Description: fmt.Sprintf("Write tests for: %s", t.Description),
				Files:       t.Files,
				ReqIDs:      t.ReqIDs,
			})
			impls = append(impls, t)
		} else {
			others = append(others, t)
		}
	}

	sort.Slice(tests, func(i, j int) bool {
		return len(tests[i].ReqIDs) > len(tests[j].ReqIDs)
	})
	sort.Slice(impls, func(i, j int) bool {
		return len(impls[i].ReqIDs) > len(impls[j].ReqIDs)
	})

	result := append(tests, impls...)
	result = append(result, others...)
	return result
}

func needsTest(t spec.Task, framework string) bool {
	lower := strings.ToLower(t.Description)
	if strings.Contains(lower, "refactor") && !strings.Contains(lower, "add") {
		return false
	}
	if strings.Contains(lower, "document") || strings.Contains(lower, "comment") {
		return false
	}
	return true
}

func rebuildTasksContent(original string, ordered []spec.Task) string {
	lines := strings.Split(original, "\n")

	reTask := regexp.MustCompile(`^(\s*-\s+\[)\s(\]\s+)(.*)$`)

	taskIndex := 0
	for i, line := range lines {
		if reTask.MatchString(line) && taskIndex < len(ordered) {
			lines[i] = fmt.Sprintf("- [ ] %s", ordered[taskIndex].Description)
			taskIndex++
		}
	}

	for taskIndex < len(ordered) {
		lines = append(lines, fmt.Sprintf("- [ ] %s", ordered[taskIndex].Description))
		taskIndex++
	}

	return strings.Join(lines, "\n")
}

func init() {
	_ = SpecTestFirstTool{}
}
