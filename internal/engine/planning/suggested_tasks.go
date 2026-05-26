package planning

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// SuggestedTask represents a proactive task suggestion based on repository state.
type SuggestedTask struct {
	ID          string
	Title       string
	Description string
	Priority    int    // 1=highest (critical), 5=lowest (nice-to-have)
	Category    string // "fix", "improve", "test", "docs", "cleanup", "security"
	Source      string // "git", "lint", "test", "todo", "pr"
	Actionable  bool
	Command     string // suggested hawk command to run
}

// TaskQueue manages a queue of suggested tasks with dismissal tracking.
type TaskQueue struct {
	Tasks     []*SuggestedTask
	Dismissed []string
	mu        sync.RWMutex
}

// NewTaskQueue creates a new empty TaskQueue.
func NewTaskQueue() *TaskQueue {
	return &TaskQueue{
		Tasks:     make([]*SuggestedTask, 0),
		Dismissed: make([]string, 0),
	}
}

// generateTaskID creates a random hex ID for a task.
func generateTaskID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Scan scans the project directory for actionable items from multiple sources.
func (tq *TaskQueue) Scan(projectDir string) ([]*SuggestedTask, error) {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	var allTasks []*SuggestedTask

	// Scan git state
	gitTasks := ScanGitTasks(projectDir)
	allTasks = append(allTasks, gitTasks...)

	// Scan TODOs
	todoTasks := ScanTODOs(projectDir)
	allTasks = append(allTasks, todoTasks...)

	// Scan test failures
	testTasks := ScanTestFailures(projectDir)
	allTasks = append(allTasks, testTasks...)

	// Scan for undocumented exports (docs)
	docTasks := scanDocsTasks(projectDir)
	allTasks = append(allTasks, docTasks...)

	// Scan for security issues
	secTasks := scanSecurityTasks(projectDir)
	allTasks = append(allTasks, secTasks...)

	tq.Tasks = allTasks
	return allTasks, nil
}

// ScanGitTasks scans the git repository for actionable git-related tasks.
func ScanGitTasks(projectDir string) []*SuggestedTask {
	var tasks []*SuggestedTask

	// Check for uncommitted changes
	cmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	cmd.Dir = projectDir
	out, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		tasks = append(tasks, &SuggestedTask{
			ID:          generateTaskID(),
			Title:       fmt.Sprintf("Commit or stash %d pending changes", len(lines)),
			Description: "There are uncommitted changes in the working tree.",
			Priority:    2,
			Category:    "cleanup",
			Source:      "git",
			Actionable:  true,
			Command:     `hawk exec "commit the pending changes with a descriptive message"`,
		})
	}

	// Check for merge conflicts
	cmd = exec.CommandContext(context.Background(), "git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = projectDir
	out, err = cmd.Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		conflictFiles := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, f := range conflictFiles {
			if f == "" {
				continue
			}
			tasks = append(tasks, &SuggestedTask{
				ID:          generateTaskID(),
				Title:       fmt.Sprintf("Resolve merge conflict in %s", f),
				Description: fmt.Sprintf("File %s has unresolved merge conflicts.", f),
				Priority:    1,
				Category:    "fix",
				Source:      "git",
				Actionable:  true,
				Command:     fmt.Sprintf(`hawk exec "resolve the merge conflict in %s"`, f),
			})
		}
	}

	// Check if behind remote
	cmd = exec.CommandContext(context.Background(), "git", "rev-list", "--count", "HEAD..@{upstream}")
	cmd.Dir = projectDir
	out, err = cmd.Output()
	if err == nil {
		count := strings.TrimSpace(string(out))
		if count != "0" && count != "" {
			tasks = append(tasks, &SuggestedTask{
				ID:          generateTaskID(),
				Title:       fmt.Sprintf("Pull latest changes (%s commits behind)", count),
				Description: "The local branch is behind the remote tracking branch.",
				Priority:    2,
				Category:    "improve",
				Source:      "git",
				Actionable:  true,
				Command:     `hawk exec "pull the latest changes from remote"`,
			})
		}
	}

	// Check for stale branches (merged branches that haven't been deleted)
	cmd = exec.CommandContext(context.Background(), "git", "branch", "--merged", "HEAD")
	cmd.Dir = projectDir
	out, err = cmd.Output()
	if err == nil {
		var staleBranches []string
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			branch := strings.TrimSpace(line)
			// Skip current branch (marked with *), main, master, develop
			if branch == "" || strings.HasPrefix(branch, "*") {
				continue
			}
			if branch == "main" || branch == "master" || branch == "develop" {
				continue
			}
			staleBranches = append(staleBranches, branch)
		}
		if len(staleBranches) > 0 {
			if len(staleBranches) == 1 {
				tasks = append(tasks, &SuggestedTask{
					ID:          generateTaskID(),
					Title:       fmt.Sprintf("Clean up stale branch: %s", staleBranches[0]),
					Description: "This merged branch can be safely deleted.",
					Priority:    4,
					Category:    "cleanup",
					Source:      "git",
					Actionable:  true,
					Command:     `hawk exec "clean up stale git branches"`,
				})
			} else {
				tasks = append(tasks, &SuggestedTask{
					ID:          generateTaskID(),
					Title:       fmt.Sprintf("%d stale branches can be deleted", len(staleBranches)),
					Description: fmt.Sprintf("Merged branches: %s", strings.Join(staleBranches, ", ")),
					Priority:    4,
					Category:    "improve",
					Source:      "git",
					Actionable:  true,
					Command:     `hawk exec "clean up stale git branches"`,
				})
			}
		}
	}

	return tasks
}

// ScanTODOs walks source files looking for TODO, FIXME, and HACK comments.
func ScanTODOs(projectDir string) []*SuggestedTask {
	var tasks []*SuggestedTask

	// File extensions to scan
	extensions := map[string]bool{
		".go":   true,
		".js":   true,
		".ts":   true,
		".py":   true,
		".rb":   true,
		".rs":   true,
		".java": true,
		".c":    true,
		".cpp":  true,
		".h":    true,
		".tsx":  true,
		".jsx":  true,
	}

	// Directories to skip
	skipDirs := map[string]bool{
		"vendor":       true,
		"node_modules": true,
		".git":         true,
		"dist":         true,
		"build":        true,
		"target":       true,
		"__pycache__":  true,
	}

	markers := []string{"TODO", "FIXME", "HACK"}

	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if !extensions[ext] {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			for _, marker := range markers {
				if strings.Contains(line, marker) {
					relPath, _ := filepath.Rel(projectDir, path)
					if relPath == "" {
						relPath = path
					}

					priority := 3
					category := "cleanup"
					if marker == "FIXME" {
						priority = 2
						category = "fix"
					} else if marker == "HACK" {
						priority = 2
						category = "cleanup"
					}

					// Extract the comment text after the marker
					idx := strings.Index(line, marker)
					comment := strings.TrimSpace(line[idx+len(marker):])
					comment = strings.TrimLeft(comment, ":( ")
					if len(comment) > 80 {
						comment = comment[:80] + "..."
					}

					title := fmt.Sprintf("Remove %s at %s:%d", marker, relPath, lineNum)
					if comment != "" {
						title = fmt.Sprintf("%s: %s", marker, comment)
						if len(title) > 80 {
							title = title[:80] + "..."
						}
					}

					tasks = append(tasks, &SuggestedTask{
						ID:          generateTaskID(),
						Title:       title,
						Description: fmt.Sprintf("%s comment at %s:%d", marker, relPath, lineNum),
						Priority:    priority,
						Category:    category,
						Source:      "todo",
						Actionable:  true,
						Command:     fmt.Sprintf(`hawk exec "implement the %s at %s:%d"`, marker, relPath, lineNum),
					})
					break // Only count one marker per line
				}
			}
		}
		return nil
	})

	return tasks
}

// ScanTestFailures runs a quick test check and generates tasks for failures.
func ScanTestFailures(projectDir string) []*SuggestedTask {
	var tasks []*SuggestedTask

	// Detect project type and run appropriate test command
	var cmd *exec.Cmd

	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err == nil {
		// Go project — run tests with short flag and timeout
		cmd = exec.CommandContext(context.Background(), "go", "test", "-short", "-timeout", "30s", "-json", "./...")
		cmd.Dir = projectDir
	} else if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err == nil {
		// Node project
		cmd = exec.CommandContext(context.Background(), "npx", "jest", "--passWithNoTests", "--no-coverage", "--json")
		cmd.Dir = projectDir
	} else if _, err := os.Stat(filepath.Join(projectDir, "Cargo.toml")); err == nil {
		// Rust project
		cmd = exec.CommandContext(context.Background(), "cargo", "test", "--no-run")
		cmd.Dir = projectDir
	} else {
		return tasks
	}

	out, err := cmd.CombinedOutput()
	if err == nil {
		// All tests passed
		return tasks
	}

	// Parse output for failure details
	output := string(out)
	lines := strings.Split(output, "\n")

	failedTests := make(map[string]bool)
	for _, line := range lines {
		// Go test JSON output: look for "Action":"fail" with Test field
		if strings.Contains(line, `"Action":"fail"`) && strings.Contains(line, `"Test":"`) {
			// Extract test name
			idx := strings.Index(line, `"Test":"`)
			if idx >= 0 {
				rest := line[idx+8:]
				end := strings.Index(rest, `"`)
				if end > 0 {
					testName := rest[:end]
					failedTests[testName] = true
				}
			}
		}
		// Fallback: look for "--- FAIL:" pattern
		if strings.Contains(line, "--- FAIL:") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				testName := parts[2]
				failedTests[testName] = true
			}
		}
	}

	for testName := range failedTests {
		tasks = append(tasks, &SuggestedTask{
			ID:          generateTaskID(),
			Title:       fmt.Sprintf("Fix failing %s", testName),
			Description: fmt.Sprintf("Test %s is failing.", testName),
			Priority:    2,
			Category:    "test",
			Source:      "test",
			Actionable:  true,
			Command:     fmt.Sprintf(`hawk exec "fix the failing test %s"`, testName),
		})
	}

	// If we detected failures but couldn't parse test names
	if len(failedTests) == 0 && err != nil {
		tasks = append(tasks, &SuggestedTask{
			ID:          generateTaskID(),
			Title:       "Fix failing tests",
			Description: "Test suite has failures that need attention.",
			Priority:    2,
			Category:    "test",
			Source:      "test",
			Actionable:  true,
			Command:     `hawk exec "fix the failing tests"`,
		})
	}

	return tasks
}

// scanDocsTasks finds exported Go functions without documentation.
func scanDocsTasks(projectDir string) []*SuggestedTask {
	var tasks []*SuggestedTask

	// Only scan Go files for exported functions without doc comments
	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err != nil {
		return tasks
	}

	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}
		// Skip test files
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		prevLineComment := false
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// Track if previous line was a comment
			isComment := strings.HasPrefix(trimmed, "//")

			// Check for exported function declarations without preceding comment
			if strings.HasPrefix(trimmed, "func ") && !prevLineComment {
				// Extract function name
				rest := trimmed[5:]
				// Skip methods (start with "(")
				if strings.HasPrefix(rest, "(") {
					prevLineComment = isComment
					continue
				}
				// Find function name
				parenIdx := strings.Index(rest, "(")
				if parenIdx > 0 {
					funcName := rest[:parenIdx]
					// Check if exported (starts with uppercase)
					if len(funcName) > 0 && funcName[0] >= 'A' && funcName[0] <= 'Z' {
						relPath, _ := filepath.Rel(projectDir, path)
						if relPath == "" {
							relPath = path
						}
						tasks = append(tasks, &SuggestedTask{
							ID:          generateTaskID(),
							Title:       fmt.Sprintf("Document exported function %s", funcName),
							Description: fmt.Sprintf("Function %s in %s lacks a doc comment.", funcName, relPath),
							Priority:    4,
							Category:    "docs",
							Source:      "lint",
							Actionable:  true,
							Command:     fmt.Sprintf(`hawk exec "add godoc to %s"`, funcName),
						})
					}
				}
			}

			prevLineComment = isComment
		}
		return nil
	})

	// Limit docs tasks to avoid noise
	if len(tasks) > 10 {
		tasks = tasks[:10]
	}

	return tasks
}

// scanSecurityTasks looks for potential security issues in the codebase.
func scanSecurityTasks(projectDir string) []*SuggestedTask {
	var tasks []*SuggestedTask

	// Check for common security issues
	securityPatterns := []struct {
		pattern     string
		title       string
		description string
		extensions  []string
	}{
		{
			pattern:     "password",
			title:       "Potential hardcoded password",
			description: "Found possible hardcoded password.",
			extensions:  []string{".go", ".py", ".js", ".ts", ".java"},
		},
		{
			pattern:     "SECRET_KEY",
			title:       "Hardcoded secret key",
			description: "Found hardcoded secret key.",
			extensions:  []string{".go", ".py", ".js", ".ts", ".java", ".env"},
		},
		{
			pattern:     "api_key",
			title:       "Potential hardcoded API key",
			description: "Found possible hardcoded API key.",
			extensions:  []string{".go", ".py", ".js", ".ts", ".java"},
		},
	}

	skipDirs := map[string]bool{
		"vendor":       true,
		"node_modules": true,
		".git":         true,
		"dist":         true,
		"build":        true,
		"target":       true,
	}

	foundIssues := make(map[string]bool)

	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip test files and generated files
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "generated_") {
			return nil
		}

		ext := filepath.Ext(path)

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer func() { _ = f.Close() }()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.ToLower(scanner.Text())
			for _, sp := range securityPatterns {
				// Check extension match
				matched := false
				for _, e := range sp.extensions {
					if ext == e {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}

				if strings.Contains(line, sp.pattern) {
					// Skip comments that just mention the word
					trimmed := strings.TrimSpace(line)
					if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
						continue
					}

					key := sp.pattern + ":" + path
					if foundIssues[key] {
						continue
					}
					foundIssues[key] = true

					relPath, _ := filepath.Rel(projectDir, path)
					if relPath == "" {
						relPath = path
					}

					tasks = append(tasks, &SuggestedTask{
						ID:          generateTaskID(),
						Title:       fmt.Sprintf("%s in %s", sp.title, relPath),
						Description: sp.description,
						Priority:    1,
						Category:    "security",
						Source:      "lint",
						Actionable:  true,
						Command:     fmt.Sprintf(`hawk exec "review %s for hardcoded secrets"`, relPath),
					})
				}
			}
		}
		return nil
	})

	// Limit security tasks
	if len(tasks) > 10 {
		tasks = tasks[:10]
	}

	return tasks
}

// Dismiss marks a task as dismissed so it won't appear in future GetTop calls.
func (tq *TaskQueue) Dismiss(taskID string) {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.Dismissed = append(tq.Dismissed, taskID)
}

// isDismissed checks if a task ID has been dismissed.
func (tq *TaskQueue) isDismissed(taskID string) bool {
	for _, id := range tq.Dismissed {
		if id == taskID {
			return true
		}
	}
	return false
}

// GetTop returns the top N suggested tasks by priority, excluding dismissed ones.
func (tq *TaskQueue) GetTop(n int) []*SuggestedTask {
	tq.mu.RLock()
	defer tq.mu.RUnlock()

	// Filter out dismissed tasks
	var active []*SuggestedTask
	for _, task := range tq.Tasks {
		if !tq.isDismissed(task.ID) {
			active = append(active, task)
		}
	}

	// Sort by priority (lower number = higher priority)
	sort.Slice(active, func(i, j int) bool {
		if active[i].Priority != active[j].Priority {
			return active[i].Priority < active[j].Priority
		}
		// Secondary sort by category importance
		catOrder := map[string]int{
			"fix":      0,
			"security": 1,
			"test":     2,
			"cleanup":  3,
			"improve":  4,
			"docs":     5,
		}
		return catOrder[active[i].Category] < catOrder[active[j].Category]
	})

	if n > len(active) {
		n = len(active)
	}
	return active[:n]
}

// FormatTasks formats a list of suggested tasks for display.
func FormatTasks(tasks []*SuggestedTask) string {
	if len(tasks) == 0 {
		return "No suggested tasks."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Suggested Tasks (%d):\n", len(tasks)))
	sb.WriteString(strings.Repeat("─", 40))
	sb.WriteString("\n")

	for i, task := range tasks {
		// Priority indicator
		var indicator string
		switch {
		case task.Priority <= 1:
			indicator = "\U0001f534" // red circle
		case task.Priority <= 3:
			indicator = "\U0001f7e1" // yellow circle
		default:
			indicator = "\U0001f535" // blue circle
		}

		sb.WriteString(fmt.Sprintf("%d. %s [%s] %s\n", i+1, indicator, task.Category, task.Title))
		if task.Command != "" {
			sb.WriteString(fmt.Sprintf("   Run: %s\n", task.Command))
		}
		if i < len(tasks)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Refresh rescans the project directory and updates the task queue.
func (tq *TaskQueue) Refresh(projectDir string) error {
	_, err := tq.Scan(projectDir)
	return err
}
