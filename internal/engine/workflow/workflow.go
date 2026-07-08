package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Workflow represents a multi-step automated workflow definition.
type Workflow struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Steps       []WorkflowStep    `json:"steps"`
	Variables   map[string]string `json:"variables"`
	OnFailure   string            `json:"on_failure"` // "abort", "continue", "retry"
	MaxDuration time.Duration     `json:"max_duration"`
	CreatedAt   time.Time         `json:"created_at"`
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	Name       string        `json:"name"`
	Action     string        `json:"action"` // "prompt", "bash", "edit", "read", "condition", "loop"
	Input      string        `json:"input"`  // template with {{.Variable}} substitution
	Output     string        `json:"output"` // variable name to store result
	Condition  string        `json:"condition"`
	MaxRetries int           `json:"max_retries"`
	Timeout    time.Duration `json:"timeout"`
	DependsOn  []string      `json:"depends_on"`
}

// WorkflowResult holds the result of a workflow execution.
type WorkflowResult struct {
	Status    string            `json:"status"` // "success", "failed", "aborted"
	Steps     []StepResult      `json:"steps"`
	Duration  time.Duration     `json:"duration"`
	Variables map[string]string `json:"variables"`
}

// StepResult holds the result of a single step execution.
type StepResult struct {
	StepName string        `json:"step_name"`
	Status   string        `json:"status"` // "success", "failed", "skipped"
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error"`
}

// WorkflowEngine manages and executes workflows.
type WorkflowEngine struct {
	Workflows map[string]*Workflow
	ExecuteFn func(ctx context.Context, action, input string) (string, error)
	mu        sync.RWMutex
}

// NewWorkflowEngine creates a new WorkflowEngine with the given execution function.
func NewWorkflowEngine(executeFn func(context.Context, string, string) (string, error)) *WorkflowEngine {
	return &WorkflowEngine{
		Workflows: make(map[string]*Workflow),
		ExecuteFn: executeFn,
	}
}

// LoadWorkflow parses a JSON workflow file and validates the step dependencies form a DAG.
func (we *WorkflowEngine) LoadWorkflow(path string) (*Workflow, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var wf Workflow
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse workflow file: %w", err)
	}

	if wf.CreatedAt.IsZero() {
		wf.CreatedAt = time.Now()
	}

	if wf.Variables == nil {
		wf.Variables = make(map[string]string)
	}

	if wf.OnFailure == "" {
		wf.OnFailure = "abort"
	}

	// Validate DAG
	if hasCycle(wf.Steps) {
		return nil, fmt.Errorf("workflow has circular dependencies")
	}

	we.mu.Lock()
	we.Workflows[wf.Name] = &wf
	we.mu.Unlock()

	return &wf, nil
}

// Execute runs a workflow to completion, resolving dependencies and executing steps.
func (we *WorkflowEngine) Execute(ctx context.Context, workflow *Workflow) (*WorkflowResult, error) {
	start := time.Now()

	// Apply MaxDuration timeout if set
	if workflow.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, workflow.MaxDuration)
		defer cancel()
	}

	result := &WorkflowResult{
		Status:    "success",
		Steps:     make([]StepResult, 0, len(workflow.Steps)),
		Variables: make(map[string]string),
	}

	// Copy initial variables
	for k, v := range workflow.Variables {
		result.Variables[k] = v
	}

	// Resolve topological order
	order, err := topologicalSort(workflow.Steps)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve step order: %w", err)
	}

	// Track completed steps for dependency resolution
	completed := make(map[string]bool)

	for _, stepIdx := range order {
		step := workflow.Steps[stepIdx]

		// Check context cancellation
		select {
		case <-ctx.Done():
			result.Status = "aborted"
			result.Duration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		// Check condition
		if step.Condition != "" {
			if !EvalCondition(step.Condition, result.Variables) {
				result.Steps = append(result.Steps, StepResult{
					StepName: step.Name,
					Status:   "skipped",
				})
				completed[step.Name] = true
				continue
			}
		}

		// Check dependencies are all completed
		allDepsCompleted := true
		for _, dep := range step.DependsOn {
			if !completed[dep] {
				allDepsCompleted = false
				break
			}
		}
		if !allDepsCompleted {
			result.Steps = append(result.Steps, StepResult{
				StepName: step.Name,
				Status:   "skipped",
				Error:    "dependencies not met",
			})
			continue
		}

		// Execute step with retries
		stepResult := we.executeStep(ctx, step, result.Variables)
		result.Steps = append(result.Steps, stepResult)

		if stepResult.Status == "success" {
			completed[step.Name] = true
			// Store output in variables if specified
			if step.Output != "" {
				result.Variables[step.Output] = stepResult.Output
			}
		} else {
			switch workflow.OnFailure {
			case "abort":
				result.Status = "failed"
				result.Duration = time.Since(start)
				return result, nil
			case "continue":
				completed[step.Name] = true
				// continue to next step
			case "retry":
				// retry is already handled in executeStep; if it still failed, abort
				result.Status = "failed"
				result.Duration = time.Since(start)
				return result, nil
			default:
				result.Status = "failed"
				result.Duration = time.Since(start)
				return result, nil
			}
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// executeStep executes a single workflow step, handling retries and timeouts.
func (we *WorkflowEngine) executeStep(ctx context.Context, step WorkflowStep, vars map[string]string) StepResult {
	start := time.Now()
	maxRetries := step.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Apply step-level timeout
		execCtx := ctx
		if step.Timeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, step.Timeout)
			defer cancel()
		}

		// Substitute variables in input
		input := SubstituteVars(step.Input, vars)

		output, err := we.ExecuteFn(execCtx, step.Action, input)
		if err == nil {
			return StepResult{
				StepName: step.Name,
				Status:   "success",
				Output:   output,
				Duration: time.Since(start),
			}
		}
		lastErr = err
	}

	errMsg := ""
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	return StepResult{
		StepName: step.Name,
		Status:   "failed",
		Duration: time.Since(start),
		Error:    errMsg,
	}
}

// SubstituteVars replaces {{.varName}} placeholders in the template with values from vars.
func SubstituteVars(template string, vars map[string]string) string {
	re := regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)
	return re.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable name
		inner := re.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		name := inner[1]
		if val, ok := vars[name]; ok {
			return val
		}
		return match
	})
}

// EvalCondition evaluates a simple condition expression with variable substitution.
// Supports: == != > < >= <= comparisons after substitution.
func EvalCondition(condition string, vars map[string]string) bool {
	// First substitute variables
	resolved := SubstituteVars(condition, vars)

	// Try equality: X == Y
	if parts := splitOp(resolved, "=="); parts != nil {
		return strings.TrimSpace(parts[0]) == strings.TrimSpace(parts[1])
	}

	// Try inequality: X != Y
	if parts := splitOp(resolved, "!="); parts != nil {
		return strings.TrimSpace(parts[0]) != strings.TrimSpace(parts[1])
	}

	// Try >=
	if parts := splitOp(resolved, ">="); parts != nil {
		return compareNumeric(parts[0], parts[1]) >= 0
	}

	// Try <=
	if parts := splitOp(resolved, "<="); parts != nil {
		return compareNumeric(parts[0], parts[1]) <= 0
	}

	// Try >
	if parts := splitOp(resolved, ">"); parts != nil {
		return compareNumeric(parts[0], parts[1]) > 0
	}

	// Try <
	if parts := splitOp(resolved, "<"); parts != nil {
		return compareNumeric(parts[0], parts[1]) < 0
	}

	// If the resolved string is non-empty and not "false" or "0", treat as truthy
	resolved = strings.TrimSpace(resolved)
	return resolved != "" && resolved != "false" && resolved != "0"
}

// splitOp splits a string by an operator, returning nil if the operator is not found.
// For operators that are prefixes of others (like < vs <=), callers must check longer ops first.
func splitOp(s, op string) []string {
	idx := strings.Index(s, op)
	if idx < 0 {
		return nil
	}
	// Avoid matching == when looking for = alone, etc.
	// Check that this is not part of a longer operator
	left := s[:idx]
	right := s[idx+len(op):]
	return []string{left, right}
}

// compareNumeric compares two string values as numbers. Returns -1, 0, or 1.
func compareNumeric(a, b string) int {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	// Strip quotes
	a = strings.Trim(a, "\"'")
	b = strings.Trim(b, "\"'")

	na, errA := strconv.ParseFloat(a, 64)
	nb, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil {
		// Fall back to string comparison
		if a < b {
			return -1
		}
		if a > b {
			return 1
		}
		return 0
	}
	if na < nb {
		return -1
	}
	if na > nb {
		return 1
	}
	return 0
}

// ValidateWorkflow checks the workflow for issues and returns a list of warnings.
func ValidateWorkflow(wf *Workflow) []string {
	var warnings []string

	if wf.Name == "" {
		warnings = append(warnings, "workflow has no name")
	}

	if len(wf.Steps) == 0 {
		warnings = append(warnings, "workflow has no steps")
	}

	// Check for duplicate step names
	stepNames := make(map[string]bool)
	for _, step := range wf.Steps {
		if step.Name == "" {
			warnings = append(warnings, "step has no name")
			continue
		}
		if stepNames[step.Name] {
			warnings = append(warnings, fmt.Sprintf("duplicate step name: %s", step.Name))
		}
		stepNames[step.Name] = true
	}

	// Check valid actions
	validActions := map[string]bool{
		"prompt": true, "bash": true, "edit": true,
		"read": true, "condition": true, "loop": true,
	}
	for _, step := range wf.Steps {
		if step.Action != "" && !validActions[step.Action] {
			warnings = append(warnings, fmt.Sprintf("step %q has unknown action: %s", step.Name, step.Action))
		}
	}

	// Check dependencies reference existing steps
	for _, step := range wf.Steps {
		for _, dep := range step.DependsOn {
			if !stepNames[dep] {
				warnings = append(warnings, fmt.Sprintf("step %q depends on unknown step: %s", step.Name, dep))
			}
		}
	}

	// Check for cycles
	if hasCycle(wf.Steps) {
		warnings = append(warnings, "dependency graph has a cycle")
	}

	// Check referenced variables in inputs are defined
	re := regexp.MustCompile(`\{\{\s*\.(\w+)\s*\}\}`)
	definedVars := make(map[string]bool)
	for k := range wf.Variables {
		definedVars[k] = true
	}
	// Also consider outputs from previous steps as defined
	for _, step := range wf.Steps {
		if step.Output != "" {
			definedVars[step.Output] = true
		}
	}
	for _, step := range wf.Steps {
		matches := re.FindAllStringSubmatch(step.Input, -1)
		for _, m := range matches {
			if len(m) >= 2 && !definedVars[m[1]] {
				warnings = append(warnings, fmt.Sprintf("step %q references undefined variable: %s", step.Name, m[1]))
			}
		}
	}

	// Check OnFailure value
	if wf.OnFailure != "" && wf.OnFailure != "abort" && wf.OnFailure != "continue" && wf.OnFailure != "retry" {
		warnings = append(warnings, fmt.Sprintf("invalid on_failure value: %s", wf.OnFailure))
	}

	return warnings
}

// hasCycle detects whether the step dependencies contain a cycle.
func hasCycle(steps []WorkflowStep) bool {
	// Build adjacency map
	nameToIdx := make(map[string]int)
	for i, s := range steps {
		nameToIdx[s.Name] = i
	}

	// State: 0 = unvisited, 1 = visiting, 2 = visited
	state := make([]int, len(steps))

	var dfs func(idx int) bool
	dfs = func(idx int) bool {
		if state[idx] == 1 {
			return true // cycle
		}
		if state[idx] == 2 {
			return false
		}
		state[idx] = 1
		for _, dep := range steps[idx].DependsOn {
			if depIdx, ok := nameToIdx[dep]; ok {
				if dfs(depIdx) {
					return true
				}
			}
		}
		state[idx] = 2
		return false
	}

	for i := range steps {
		if state[i] == 0 {
			if dfs(i) {
				return true
			}
		}
	}
	return false
}

// topologicalSort returns step indices in dependency-resolved order (Kahn's algorithm).
func topologicalSort(steps []WorkflowStep) ([]int, error) {
	n := len(steps)
	nameToIdx := make(map[string]int)
	for i, s := range steps {
		nameToIdx[s.Name] = i
	}

	// Compute in-degree
	inDegree := make([]int, n)
	adjList := make([][]int, n) // adjList[i] lists steps that depend on i
	for i, step := range steps {
		for _, dep := range step.DependsOn {
			if depIdx, ok := nameToIdx[dep]; ok {
				adjList[depIdx] = append(adjList[depIdx], i)
				inDegree[i]++
			}
		}
	}

	// Start with nodes that have no dependencies
	var queue []int
	for i := 0; i < n; i++ {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}

	var order []int
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		order = append(order, curr)
		for _, next := range adjList[curr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if len(order) != n {
		return nil, fmt.Errorf("cycle detected in dependency graph")
	}
	return order, nil
}

// BuiltinWorkflows returns the built-in workflow templates.
func BuiltinWorkflows() map[string]*Workflow {
	return map[string]*Workflow{
		"pr-review": {
			Name:        "pr-review",
			Description: "Review a pull request: read diff, analyze, post findings",
			Variables:   map[string]string{"branch": "main"},
			OnFailure:   "abort",
			Steps: []WorkflowStep{
				{
					Name:   "read-diff",
					Action: "bash",
					Input:  "git diff {{.branch}}...HEAD",
					Output: "diff",
				},
				{
					Name:      "analyze",
					Action:    "prompt",
					Input:     "Analyze this diff for issues, security concerns, and style:\n{{.diff}}",
					Output:    "analysis",
					DependsOn: []string{"read-diff"},
				},
				{
					Name:      "post-findings",
					Action:    "prompt",
					Input:     "Summarize the review findings:\n{{.analysis}}",
					Output:    "summary",
					DependsOn: []string{"analyze"},
				},
			},
		},
		"fix-tests": {
			Name:        "fix-tests",
			Description: "Run tests, find failures, fix them, and rerun",
			Variables:   map[string]string{"test_cmd": "go test ./..."},
			OnFailure:   "continue",
			Steps: []WorkflowStep{
				{
					Name:   "run-tests",
					Action: "bash",
					Input:  "{{.test_cmd}}",
					Output: "test_output",
				},
				{
					Name:      "find-failures",
					Action:    "prompt",
					Input:     "Identify the failing tests and their root causes:\n{{.test_output}}",
					Output:    "failures",
					DependsOn: []string{"run-tests"},
				},
				{
					Name:       "fix",
					Action:     "prompt",
					Input:      "Fix the following test failures:\n{{.failures}}",
					Output:     "fix_result",
					DependsOn:  []string{"find-failures"},
					MaxRetries: 2,
				},
				{
					Name:      "rerun-tests",
					Action:    "bash",
					Input:     "{{.test_cmd}}",
					Output:    "rerun_output",
					DependsOn: []string{"fix"},
				},
			},
		},
		"refactor": {
			Name:        "refactor",
			Description: "Analyze code, plan refactoring, implement, test, and commit",
			Variables:   map[string]string{"target": ".", "test_cmd": "go test ./..."},
			OnFailure:   "abort",
			Steps: []WorkflowStep{
				{
					Name:   "analyze",
					Action: "prompt",
					Input:  "Analyze the code in {{.target}} for refactoring opportunities",
					Output: "analysis",
				},
				{
					Name:      "plan",
					Action:    "prompt",
					Input:     "Create a detailed refactoring plan based on:\n{{.analysis}}",
					Output:    "plan",
					DependsOn: []string{"analyze"},
				},
				{
					Name:      "implement",
					Action:    "prompt",
					Input:     "Implement the following refactoring plan:\n{{.plan}}",
					Output:    "impl_result",
					DependsOn: []string{"plan"},
				},
				{
					Name:      "test",
					Action:    "bash",
					Input:     "{{.test_cmd}}",
					Output:    "test_result",
					DependsOn: []string{"implement"},
				},
				{
					Name:      "commit",
					Action:    "bash",
					Input:     "git add -A && git commit -m \"refactor: apply planned improvements\"",
					Output:    "commit_result",
					DependsOn: []string{"test"},
					Condition: "{{.test_result}} == ",
				},
			},
		},
	}
}

// FormatResult produces a human-readable summary of a workflow result.
func FormatResult(result *WorkflowResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Workflow Status: %s\n", result.Status))
	sb.WriteString(fmt.Sprintf("Duration: %s\n", result.Duration.Round(time.Millisecond)))
	sb.WriteString(fmt.Sprintf("Steps: %d\n", len(result.Steps)))
	sb.WriteString(strings.Repeat("-", 50))
	sb.WriteString("\n")

	for i, step := range result.Steps {
		sb.WriteString(fmt.Sprintf("  %d. [%s] %s", i+1, step.Status, step.StepName))
		if step.Duration > 0 {
			sb.WriteString(fmt.Sprintf(" (%s)", step.Duration.Round(time.Millisecond)))
		}
		if step.Error != "" {
			sb.WriteString(fmt.Sprintf("\n     Error: %s", step.Error))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(strings.Repeat("-", 50))
	sb.WriteString("\n")

	if len(result.Variables) > 0 {
		sb.WriteString("Variables:\n")
		for k, v := range result.Variables {
			display := v
			if len(display) > 80 {
				display = display[:77] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %s = %s\n", k, display))
		}
	}

	return sb.String()
}
