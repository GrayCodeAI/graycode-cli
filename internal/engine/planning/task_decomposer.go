package planning

import (
	"fmt"
	"strings"
	"sync"
)

// Task represents a single unit of work within a decomposed plan.
type Task struct {
	ID              string
	Description     string
	Type            string // "read", "analyze", "plan", "implement", "test", "review"
	Dependencies    []string
	EstimatedTokens int
	Priority        int
	Status          string // "pending", "running", "done", "failed"
	Result          string
}

// TaskPlan holds a full decomposition of a goal into ordered tasks.
type TaskPlan struct {
	Goal           string
	Tasks          []Task
	EstimatedTotal int
	Parallel       [][]string // groups of task IDs that can run concurrently
}

// TaskDecomposer breaks complex goals into ordered sub-tasks.
type TaskDecomposer struct {
	MaxTasks int
	mu       sync.Mutex
}

// NewTaskDecomposer creates a TaskDecomposer with default settings.
func NewTaskDecomposer() *TaskDecomposer {
	return &TaskDecomposer{
		MaxTasks: 10,
	}
}

// Decompose analyzes a goal and returns a structured TaskPlan.
func (td *TaskDecomposer) Decompose(goal string) *TaskPlan {
	td.mu.Lock()
	defer td.mu.Unlock()

	pattern := td.DetectPattern(goal)

	var tasks []Task
	switch pattern {
	case "implementation":
		tasks = td.ImplementationPlan(goal)
	case "debugging":
		tasks = td.DebuggingPlan(goal)
	case "refactoring":
		tasks = td.RefactoringPlan(goal)
	case "testing":
		tasks = td.TestingPlan(goal)
	case "review":
		tasks = td.reviewPlan(goal)
	case "explanation":
		tasks = td.explanationPlan(goal)
	default:
		tasks = td.ImplementationPlan(goal)
	}

	// Enforce MaxTasks limit
	if len(tasks) > td.MaxTasks {
		tasks = tasks[:td.MaxTasks]
	}

	total := 0
	for _, t := range tasks {
		total += t.EstimatedTokens
	}

	parallel := td.FindParallelGroups(tasks)

	return &TaskPlan{
		Goal:           goal,
		Tasks:          tasks,
		EstimatedTotal: total,
		Parallel:       parallel,
	}
}

// DetectPattern determines the type of work from goal keywords.
func (td *TaskDecomposer) DetectPattern(goal string) string {
	lower := strings.ToLower(goal)

	patterns := []struct {
		keywords []string
		name     string
	}{
		{[]string{"implement", "add feature", "create", "build"}, "implementation"},
		{[]string{"fix", "bug", "error", "broken", "crash"}, "debugging"},
		{[]string{"refactor", "restructure", "reorganize", "clean up"}, "refactoring"},
		{[]string{"review", "audit", "inspect"}, "review"},
		{[]string{"test", "coverage", "spec"}, "testing"},
		{[]string{"explain", "document", "describe"}, "explanation"},
	}

	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, kw) {
				return p.name
			}
		}
	}

	return "implementation"
}

// ImplementationPlan returns the task sequence for implementing a feature.
func (td *TaskDecomposer) ImplementationPlan(goal string) []Task {
	complexity := td.EstimateComplexity(goal)
	base := 1000 + complexity*100

	return []Task{
		{
			ID:              "task-1",
			Description:     "Understand existing code related to: " + goal,
			Type:            "read",
			Dependencies:    nil,
			EstimatedTokens: base,
			Priority:        1,
			Status:          "pending",
		},
		{
			ID:              "task-2",
			Description:     "Identify integration points and constraints",
			Type:            "analyze",
			Dependencies:    []string{"task-1"},
			EstimatedTokens: base - 500,
			Priority:        2,
			Status:          "pending",
		},
		{
			ID:              "task-3",
			Description:     "Plan implementation approach for: " + goal,
			Type:            "plan",
			Dependencies:    []string{"task-2"},
			EstimatedTokens: base + 500,
			Priority:        3,
			Status:          "pending",
		},
		{
			ID:              "task-4",
			Description:     "Implement the solution",
			Type:            "implement",
			Dependencies:    []string{"task-3"},
			EstimatedTokens: base + 1500,
			Priority:        4,
			Status:          "pending",
		},
		{
			ID:              "task-5",
			Description:     "Write tests for the implementation",
			Type:            "test",
			Dependencies:    []string{"task-4"},
			EstimatedTokens: base,
			Priority:        5,
			Status:          "pending",
		},
	}
}

// DebuggingPlan returns the task sequence for fixing a bug.
func (td *TaskDecomposer) DebuggingPlan(goal string) []Task {
	complexity := td.EstimateComplexity(goal)
	base := 800 + complexity*80

	return []Task{
		{
			ID:              "task-1",
			Description:     "Reproduce the issue: " + goal,
			Type:            "read",
			Dependencies:    nil,
			EstimatedTokens: base,
			Priority:        1,
			Status:          "pending",
		},
		{
			ID:              "task-2",
			Description:     "Investigate root cause through code and logs",
			Type:            "analyze",
			Dependencies:    []string{"task-1"},
			EstimatedTokens: base + 200,
			Priority:        2,
			Status:          "pending",
		},
		{
			ID:              "task-3",
			Description:     "Form hypothesis and plan fix",
			Type:            "plan",
			Dependencies:    []string{"task-2"},
			EstimatedTokens: base - 200,
			Priority:        3,
			Status:          "pending",
		},
		{
			ID:              "task-4",
			Description:     "Apply the fix",
			Type:            "implement",
			Dependencies:    []string{"task-3"},
			EstimatedTokens: base + 500,
			Priority:        4,
			Status:          "pending",
		},
		{
			ID:              "task-5",
			Description:     "Verify the fix resolves the issue",
			Type:            "test",
			Dependencies:    []string{"task-4"},
			EstimatedTokens: base,
			Priority:        5,
			Status:          "pending",
		},
	}
}

// RefactoringPlan returns the task sequence for refactoring code.
func (td *TaskDecomposer) RefactoringPlan(goal string) []Task {
	complexity := td.EstimateComplexity(goal)
	base := 900 + complexity*90

	return []Task{
		{
			ID:              "task-1",
			Description:     "Understand current structure: " + goal,
			Type:            "read",
			Dependencies:    nil,
			EstimatedTokens: base + 200,
			Priority:        1,
			Status:          "pending",
		},
		{
			ID:              "task-2",
			Description:     "Plan new structure and migration path",
			Type:            "plan",
			Dependencies:    []string{"task-1"},
			EstimatedTokens: base + 500,
			Priority:        2,
			Status:          "pending",
		},
		{
			ID:              "task-3",
			Description:     "Migrate code to new structure",
			Type:            "implement",
			Dependencies:    []string{"task-2"},
			EstimatedTokens: base + 1000,
			Priority:        3,
			Status:          "pending",
		},
		{
			ID:              "task-4",
			Description:     "Run tests and verify behavior is preserved",
			Type:            "test",
			Dependencies:    []string{"task-3"},
			EstimatedTokens: base,
			Priority:        4,
			Status:          "pending",
		},
		{
			ID:              "task-5",
			Description:     "Clean up old code and dead references",
			Type:            "review",
			Dependencies:    []string{"task-4"},
			EstimatedTokens: base - 300,
			Priority:        5,
			Status:          "pending",
		},
	}
}

// TestingPlan returns the task sequence for adding tests.
func (td *TaskDecomposer) TestingPlan(goal string) []Task {
	complexity := td.EstimateComplexity(goal)
	base := 700 + complexity*70

	return []Task{
		{
			ID:              "task-1",
			Description:     "Read source code to understand behavior: " + goal,
			Type:            "read",
			Dependencies:    nil,
			EstimatedTokens: base + 300,
			Priority:        1,
			Status:          "pending",
		},
		{
			ID:              "task-2",
			Description:     "Identify test cases and edge conditions",
			Type:            "analyze",
			Dependencies:    []string{"task-1"},
			EstimatedTokens: base,
			Priority:        2,
			Status:          "pending",
		},
		{
			ID:              "task-3",
			Description:     "Write test implementations",
			Type:            "implement",
			Dependencies:    []string{"task-2"},
			EstimatedTokens: base + 800,
			Priority:        3,
			Status:          "pending",
		},
		{
			ID:              "task-4",
			Description:     "Verify coverage and fix gaps",
			Type:            "test",
			Dependencies:    []string{"task-3"},
			EstimatedTokens: base + 200,
			Priority:        4,
			Status:          "pending",
		},
	}
}

// reviewPlan returns the task sequence for a code review or audit.
func (td *TaskDecomposer) reviewPlan(goal string) []Task {
	complexity := td.EstimateComplexity(goal)
	base := 800 + complexity*80

	return []Task{
		{
			ID:              "task-1",
			Description:     "Read through the code: " + goal,
			Type:            "read",
			Dependencies:    nil,
			EstimatedTokens: base + 500,
			Priority:        1,
			Status:          "pending",
		},
		{
			ID:              "task-2",
			Description:     "Analyze patterns, anti-patterns, and potential issues",
			Type:            "analyze",
			Dependencies:    []string{"task-1"},
			EstimatedTokens: base + 300,
			Priority:        2,
			Status:          "pending",
		},
		{
			ID:              "task-3",
			Description:     "Compile findings and recommendations",
			Type:            "review",
			Dependencies:    []string{"task-2"},
			EstimatedTokens: base + 200,
			Priority:        3,
			Status:          "pending",
		},
	}
}

// explanationPlan returns the task sequence for explaining or documenting code.
func (td *TaskDecomposer) explanationPlan(goal string) []Task {
	complexity := td.EstimateComplexity(goal)
	base := 600 + complexity*60

	return []Task{
		{
			ID:              "task-1",
			Description:     "Read and understand the code: " + goal,
			Type:            "read",
			Dependencies:    nil,
			EstimatedTokens: base + 400,
			Priority:        1,
			Status:          "pending",
		},
		{
			ID:              "task-2",
			Description:     "Analyze structure and data flow",
			Type:            "analyze",
			Dependencies:    []string{"task-1"},
			EstimatedTokens: base + 200,
			Priority:        2,
			Status:          "pending",
		},
		{
			ID:              "task-3",
			Description:     "Write explanation or documentation",
			Type:            "implement",
			Dependencies:    []string{"task-2"},
			EstimatedTokens: base + 600,
			Priority:        3,
			Status:          "pending",
		},
	}
}

// FindParallelGroups groups tasks by dependency level so independent tasks
// can run concurrently within the same group.
func (td *TaskDecomposer) FindParallelGroups(tasks []Task) [][]string {
	if len(tasks) == 0 {
		return nil
	}

	// Build a map from task ID to its dependency level.
	levels := make(map[string]int)
	taskMap := make(map[string]*Task)
	for i := range tasks {
		taskMap[tasks[i].ID] = &tasks[i]
	}

	// Compute level for each task (max dependency level + 1).
	var computeLevel func(id string) int
	computeLevel = func(id string) int {
		if lvl, ok := levels[id]; ok {
			return lvl
		}
		t, ok := taskMap[id]
		if !ok {
			return 0
		}
		if len(t.Dependencies) == 0 {
			levels[id] = 0
			return 0
		}
		maxDep := 0
		for _, dep := range t.Dependencies {
			depLevel := computeLevel(dep)
			if depLevel+1 > maxDep {
				maxDep = depLevel + 1
			}
		}
		levels[id] = maxDep
		return maxDep
	}

	maxLevel := 0
	for _, t := range tasks {
		lvl := computeLevel(t.ID)
		if lvl > maxLevel {
			maxLevel = lvl
		}
	}

	// Group tasks by level.
	groups := make([][]string, maxLevel+1)
	for i := range groups {
		groups[i] = []string{}
	}
	for _, t := range tasks {
		lvl := levels[t.ID]
		groups[lvl] = append(groups[lvl], t.ID)
	}

	return groups
}

// FormatPlan returns a human-readable display of the task plan.
func (td *TaskDecomposer) FormatPlan(plan *TaskPlan) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Task Plan: %q\n", plan.Goal))
	b.WriteString(strings.Repeat("═", 43) + "\n")
	b.WriteString(fmt.Sprintf("Estimated: ~%s tokens across %d tasks\n\n",
		fmtTokensComma(plan.EstimatedTotal), len(plan.Tasks)))

	for i, t := range plan.Tasks {
		deps := "none"
		if len(t.Dependencies) > 0 {
			depNums := make([]string, len(t.Dependencies))
			for j, d := range t.Dependencies {
				depNums[j] = strings.TrimPrefix(d, "task-")
			}
			deps = strings.Join(depNums, ", ")
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, t.Type, t.Description))
		b.WriteString(fmt.Sprintf("   Dependencies: %s | Est: %s tokens\n",
			deps, fmtTokensComma(t.EstimatedTokens)))
	}

	if len(plan.Parallel) > 0 {
		b.WriteString("\nParallel groups: ")
		parts := make([]string, len(plan.Parallel))
		for i, group := range plan.Parallel {
			ids := make([]string, len(group))
			for j, id := range group {
				ids[j] = strings.TrimPrefix(id, "task-")
			}
			parts[i] = "[" + strings.Join(ids, ", ") + "]"
		}
		b.WriteString(strings.Join(parts, " → "))
		b.WriteString("\n")
	}

	return b.String()
}

// EstimateComplexity scores a goal's complexity based on word count and keyword multipliers.
func (td *TaskDecomposer) EstimateComplexity(goal string) int {
	words := strings.Fields(goal)
	score := len(words)

	multipliers := map[string]int{
		"authentication": 3,
		"database":       3,
		"migration":      3,
		"distributed":    4,
		"concurrent":     3,
		"async":          2,
		"api":            2,
		"integration":    2,
		"security":       3,
		"performance":    2,
		"cache":          2,
		"caching":        2,
		"websocket":      2,
		"streaming":      2,
		"encryption":     3,
		"middleware":     2,
		"microservice":   4,
		"kubernetes":     3,
		"docker":         2,
		"deploy":         2,
		"ci":             2,
		"pipeline":       2,
	}

	lower := strings.ToLower(goal)
	for keyword, mult := range multipliers {
		if strings.Contains(lower, keyword) {
			score += mult
		}
	}

	return score
}

// fmtTokensComma formats a token count with comma separators.
func fmtTokensComma(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
		if len(s) > remainder {
			result.WriteString(",")
		}
	}
	for i := remainder; i < len(s); i += 3 {
		if i > remainder {
			result.WriteString(",")
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
