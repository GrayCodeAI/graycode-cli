package planning

import (
	"strings"
	"sync"
	"testing"
)

func TestNewTaskDecomposer(t *testing.T) {
	td := NewTaskDecomposer()
	if td == nil {
		t.Fatal("NewTaskDecomposer returned nil")
	}
	if td.MaxTasks != 10 {
		t.Errorf("MaxTasks = %d, want 10", td.MaxTasks)
	}
}

func TestDetectPattern(t *testing.T) {
	td := NewTaskDecomposer()

	tests := []struct {
		goal    string
		pattern string
	}{
		{"implement JWT authentication", "implementation"},
		{"add feature for user login", "implementation"},
		{"create a new REST endpoint", "implementation"},
		{"build a caching layer", "implementation"},
		{"fix the null pointer bug", "debugging"},
		{"there is a bug in the parser", "debugging"},
		{"error handling is broken", "debugging"},
		{"the server crashes on startup", "debugging"},
		{"refactor the database layer", "refactoring"},
		{"restructure the project layout", "refactoring"},
		{"reorganize the config package", "refactoring"},
		{"clean up the handler code", "refactoring"},
		{"add tests for the auth module", "testing"},
		{"improve test coverage", "testing"},
		{"write spec for the API", "testing"},
		{"review the pull request", "review"},
		{"audit the security code", "review"},
		{"inspect the middleware chain", "review"},
		{"explain how the router works", "explanation"},
		{"document the API endpoints", "explanation"},
		{"describe the architecture", "explanation"},
		{"do something generic", "implementation"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.goal, func(t *testing.T) {
			got := td.DetectPattern(tt.goal)
			if got != tt.pattern {
				t.Errorf("DetectPattern(%q) = %q, want %q", tt.goal, got, tt.pattern)
			}
		})
	}
}

func TestDecompose_Implementation(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("implement JWT authentication")

	if plan.Goal != "implement JWT authentication" {
		t.Errorf("Goal = %q, want %q", plan.Goal, "implement JWT authentication")
	}
	if len(plan.Tasks) != 5 {
		t.Fatalf("len(Tasks) = %d, want 5", len(plan.Tasks))
	}
	if plan.EstimatedTotal <= 0 {
		t.Error("EstimatedTotal should be positive")
	}

	// Verify task types follow implementation pattern
	expectedTypes := []string{"read", "analyze", "plan", "implement", "test"}
	for i, tt := range expectedTypes {
		if plan.Tasks[i].Type != tt {
			t.Errorf("Tasks[%d].Type = %q, want %q", i, plan.Tasks[i].Type, tt)
		}
	}

	// Verify dependencies chain
	if len(plan.Tasks[0].Dependencies) != 0 {
		t.Error("First task should have no dependencies")
	}
	for i := 1; i < len(plan.Tasks); i++ {
		if len(plan.Tasks[i].Dependencies) == 0 {
			t.Errorf("Tasks[%d] should have dependencies", i)
		}
	}

	// Verify all tasks start as pending
	for i, task := range plan.Tasks {
		if task.Status != "pending" {
			t.Errorf("Tasks[%d].Status = %q, want %q", i, task.Status, "pending")
		}
	}
}

func TestDecompose_Debugging(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("fix the null pointer bug in parser")

	if len(plan.Tasks) != 5 {
		t.Fatalf("len(Tasks) = %d, want 5", len(plan.Tasks))
	}

	// First task should be about reproducing
	if !strings.Contains(plan.Tasks[0].Description, "Reproduce") {
		t.Errorf("First task should be about reproducing, got: %s", plan.Tasks[0].Description)
	}
}

func TestDecompose_Refactoring(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("refactor the database connection pool")

	if len(plan.Tasks) != 5 {
		t.Fatalf("len(Tasks) = %d, want 5", len(plan.Tasks))
	}

	expectedTypes := []string{"read", "plan", "implement", "test", "review"}
	for i, tt := range expectedTypes {
		if plan.Tasks[i].Type != tt {
			t.Errorf("Tasks[%d].Type = %q, want %q", i, plan.Tasks[i].Type, tt)
		}
	}
}

func TestDecompose_Testing(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("add tests for the authentication module")

	if len(plan.Tasks) != 4 {
		t.Fatalf("len(Tasks) = %d, want 4", len(plan.Tasks))
	}

	// First should be reading source
	if plan.Tasks[0].Type != "read" {
		t.Errorf("Tasks[0].Type = %q, want %q", plan.Tasks[0].Type, "read")
	}
}

func TestDecompose_MaxTasks(t *testing.T) {
	td := NewTaskDecomposer()
	td.MaxTasks = 3

	plan := td.Decompose("implement a distributed caching layer")
	if len(plan.Tasks) > 3 {
		t.Errorf("len(Tasks) = %d, should be capped at MaxTasks=3", len(plan.Tasks))
	}
}

func TestFindParallelGroups_LinearChain(t *testing.T) {
	td := NewTaskDecomposer()
	tasks := []Task{
		{ID: "task-1", Dependencies: nil},
		{ID: "task-2", Dependencies: []string{"task-1"}},
		{ID: "task-3", Dependencies: []string{"task-2"}},
	}

	groups := td.FindParallelGroups(tasks)
	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3", len(groups))
	}
	for i, g := range groups {
		if len(g) != 1 {
			t.Errorf("groups[%d] has %d items, want 1", i, len(g))
		}
	}
}

func TestFindParallelGroups_Parallel(t *testing.T) {
	td := NewTaskDecomposer()
	tasks := []Task{
		{ID: "task-1", Dependencies: nil},
		{ID: "task-2", Dependencies: nil},
		{ID: "task-3", Dependencies: []string{"task-1", "task-2"}},
	}

	groups := td.FindParallelGroups(tasks)
	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}
	if len(groups[0]) != 2 {
		t.Errorf("groups[0] has %d items, want 2 (parallel tasks)", len(groups[0]))
	}
	if len(groups[1]) != 1 {
		t.Errorf("groups[1] has %d items, want 1", len(groups[1]))
	}
}

func TestFindParallelGroups_Empty(t *testing.T) {
	td := NewTaskDecomposer()
	groups := td.FindParallelGroups(nil)
	if groups != nil {
		t.Errorf("expected nil for empty tasks, got %v", groups)
	}
}

func TestTaskDecomposerEstimateComplexity(t *testing.T) {
	td := NewTaskDecomposer()

	// Simple goal
	simple := td.EstimateComplexity("fix a typo")
	// Complex goal
	complex := td.EstimateComplexity("implement distributed authentication with caching and encryption for microservice")

	if complex <= simple {
		t.Errorf("complex goal (%d) should have higher score than simple goal (%d)", complex, simple)
	}
}

func TestEstimateComplexity_Keywords(t *testing.T) {
	td := NewTaskDecomposer()

	withoutKeyword := td.EstimateComplexity("add a handler")
	withKeyword := td.EstimateComplexity("add a authentication handler")

	if withKeyword <= withoutKeyword {
		t.Errorf("keyword goal (%d) should score higher than plain goal (%d)", withKeyword, withoutKeyword)
	}
}

func TestTaskDecomposerFormatPlan(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("implement JWT authentication")

	output := td.FormatPlan(plan)

	// Check required sections
	if !strings.Contains(output, "Task Plan:") {
		t.Error("output should contain 'Task Plan:'")
	}
	if !strings.Contains(output, "JWT authentication") {
		t.Error("output should contain the goal")
	}
	if !strings.Contains(output, "═") {
		t.Error("output should contain separator line")
	}
	if !strings.Contains(output, "Estimated:") {
		t.Error("output should contain estimated tokens")
	}
	if !strings.Contains(output, "tokens across") {
		t.Error("output should contain task count")
	}
	if !strings.Contains(output, "[read]") {
		t.Error("output should contain task type indicators")
	}
	if !strings.Contains(output, "Dependencies:") {
		t.Error("output should contain dependency info")
	}
	if !strings.Contains(output, "Parallel groups:") {
		t.Error("output should contain parallel groups")
	}
}

func TestFormatPlan_ParallelGroups(t *testing.T) {
	td := NewTaskDecomposer()
	plan := &TaskPlan{
		Goal: "test parallel display",
		Tasks: []Task{
			{ID: "task-1", Type: "read", Description: "Read code", Dependencies: nil, EstimatedTokens: 500},
			{ID: "task-2", Type: "analyze", Description: "Analyze", Dependencies: nil, EstimatedTokens: 500},
			{ID: "task-3", Type: "implement", Description: "Build", Dependencies: []string{"task-1", "task-2"}, EstimatedTokens: 1000},
		},
		EstimatedTotal: 2000,
		Parallel:       [][]string{{"task-1", "task-2"}, {"task-3"}},
	}

	output := td.FormatPlan(plan)
	if !strings.Contains(output, "[1, 2]") {
		t.Errorf("output should show parallel group [1, 2], got:\n%s", output)
	}
}

func TestTaskIDs_Unique(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("implement a complete user system")

	seen := make(map[string]bool)
	for _, task := range plan.Tasks {
		if seen[task.ID] {
			t.Errorf("duplicate task ID: %s", task.ID)
		}
		seen[task.ID] = true
	}
}

func TestTaskPriorities_Ordered(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("implement something")

	for i := 1; i < len(plan.Tasks); i++ {
		if plan.Tasks[i].Priority <= plan.Tasks[i-1].Priority {
			t.Errorf("Tasks[%d].Priority (%d) should be > Tasks[%d].Priority (%d)",
				i, plan.Tasks[i].Priority, i-1, plan.Tasks[i-1].Priority)
		}
	}
}

func TestDecompose_ConcurrentSafe(t *testing.T) {
	td := NewTaskDecomposer()
	var wg sync.WaitGroup

	goals := []string{
		"implement user authentication",
		"fix the login bug",
		"refactor the database layer",
		"add tests for the API",
		"review the security module",
	}

	for _, goal := range goals {
		wg.Add(1)
		go func(g string) {
			defer wg.Done()
			plan := td.Decompose(g)
			if plan == nil {
				t.Errorf("Decompose(%q) returned nil", g)
				return
			}
			if len(plan.Tasks) == 0 {
				t.Errorf("Decompose(%q) returned empty tasks", g)
			}
		}(goal)
	}

	wg.Wait()
}

func TestFormatTokenCount(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{500, "500"},
		{1000, "1,000"},
		{8500, "8,500"},
		{12345, "12,345"},
		{100000, "100,000"},
	}

	for _, tt := range tests {
		got := fmtTokensComma(tt.input)
		if got != tt.expected {
			t.Errorf("fmtTokensComma(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDecompose_Review(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("review the authentication middleware")

	if len(plan.Tasks) != 3 {
		t.Fatalf("len(Tasks) = %d, want 3", len(plan.Tasks))
	}

	expectedTypes := []string{"read", "analyze", "review"}
	for i, tt := range expectedTypes {
		if plan.Tasks[i].Type != tt {
			t.Errorf("Tasks[%d].Type = %q, want %q", i, plan.Tasks[i].Type, tt)
		}
	}
}

func TestDecompose_Explanation(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("explain how the router dispatches requests")

	if len(plan.Tasks) != 3 {
		t.Fatalf("len(Tasks) = %d, want 3", len(plan.Tasks))
	}

	expectedTypes := []string{"read", "analyze", "implement"}
	for i, tt := range expectedTypes {
		if plan.Tasks[i].Type != tt {
			t.Errorf("Tasks[%d].Type = %q, want %q", i, plan.Tasks[i].Type, tt)
		}
	}
}

func TestEstimatedTotal_MatchesSum(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("implement distributed caching")

	sum := 0
	for _, task := range plan.Tasks {
		sum += task.EstimatedTokens
	}

	if plan.EstimatedTotal != sum {
		t.Errorf("EstimatedTotal = %d, sum of tasks = %d", plan.EstimatedTotal, sum)
	}
}

func TestDependencies_ReferenceValidTasks(t *testing.T) {
	td := NewTaskDecomposer()
	plan := td.Decompose("implement a REST API")

	ids := make(map[string]bool)
	for _, task := range plan.Tasks {
		ids[task.ID] = true
	}

	for _, task := range plan.Tasks {
		for _, dep := range task.Dependencies {
			if !ids[dep] {
				t.Errorf("Task %s depends on %s which does not exist", task.ID, dep)
			}
		}
	}
}
