package engine

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ExecutionPlan represents the full plan for executing a batch of tool calls,
// including ordering, parallelization groups, and timing estimates.
type ExecutionPlan struct {
	Steps              []ExecutionStep
	TotalEstimatedTime time.Duration
	Parallelizable     bool
	Dependencies       map[string][]string // step ID -> list of step IDs it depends on
}

// ExecutionStep represents a single tool invocation within an execution plan.
type ExecutionStep struct {
	ID                string
	ToolName          string
	Args              map[string]interface{}
	DependsOn         []string
	EstimatedDuration time.Duration
	CanParallel       bool
	Priority          int
	Group             int
}

// PlannedCall describes a tool call to be planned, including which files it targets.
type PlannedCall struct {
	ToolName string
	Args     map[string]interface{}
	Targets  []string // files affected by this call
}

// ExecutionPlanner analyzes tool calls and determines optimal execution order
// and parallelization opportunities based on dependency analysis and timing data.
type ExecutionPlanner struct {
	ToolTimings map[string]time.Duration
	mu          sync.RWMutex
}

// NewExecutionPlanner creates an ExecutionPlanner with default timing estimates
// for common tool operations.
func NewExecutionPlanner() *ExecutionPlanner {
	return &ExecutionPlanner{
		ToolTimings: map[string]time.Duration{
			"Read":     100 * time.Millisecond,
			"Edit":     200 * time.Millisecond,
			"Write":    200 * time.Millisecond,
			"Bash":     5 * time.Second,
			"Grep":     500 * time.Millisecond,
			"Glob":     200 * time.Millisecond,
			"WebFetch": 3 * time.Second,
		},
	}
}

// Plan analyzes the given tool calls for dependencies and groups them into an
// optimized execution plan that maximizes parallelism while respecting ordering
// constraints.
func (ep *ExecutionPlanner) Plan(toolCalls []PlannedCall) *ExecutionPlan {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if len(toolCalls) == 0 {
		return &ExecutionPlan{
			Dependencies: make(map[string][]string),
		}
	}

	// Build steps with IDs
	steps := make([]ExecutionStep, len(toolCalls))
	for i, call := range toolCalls {
		steps[i] = ExecutionStep{
			ID:                fmt.Sprintf("step_%d", i),
			ToolName:          call.ToolName,
			Args:              call.Args,
			EstimatedDuration: ep.estimateToolDuration(call.ToolName),
			Priority:          i,
		}
	}

	// Find dependencies
	deps := ep.FindDependencies(toolCalls)

	// Assign dependencies to steps
	for i := range steps {
		stepID := steps[i].ID
		if depList, ok := deps[stepID]; ok {
			steps[i].DependsOn = depList
		}
	}

	// Build the plan
	plan := &ExecutionPlan{
		Steps:        steps,
		Dependencies: deps,
	}

	// Group into parallel batches
	groups := ep.GroupParallel(plan)

	// Assign group numbers and CanParallel flags
	for groupIdx, group := range groups {
		for _, step := range group {
			for i := range plan.Steps {
				if plan.Steps[i].ID == step.ID {
					plan.Steps[i].Group = groupIdx + 1
					plan.Steps[i].CanParallel = len(group) > 1
				}
			}
		}
	}

	// Determine if any parallelism exists
	plan.Parallelizable = false
	for _, group := range groups {
		if len(group) > 1 {
			plan.Parallelizable = true
			break
		}
	}

	// Calculate total estimated time
	plan.TotalEstimatedTime = ep.EstimateDuration(plan)

	return plan
}

// FindDependencies builds a dependency graph from the planned calls based on
// file target analysis. A write/edit to file X creates a dependency for any
// subsequent read/edit of the same file.
func (ep *ExecutionPlanner) FindDependencies(calls []PlannedCall) map[string][]string {
	deps := make(map[string][]string)

	// Track which steps write to which files, in order
	type fileOp struct {
		stepIdx int
		op      string // "read", "write", "edit"
	}

	fileOps := make(map[string][]fileOp)

	for i, call := range calls {
		op := classifyOp(call.ToolName)
		for _, target := range call.Targets {
			fileOps[target] = append(fileOps[target], fileOp{stepIdx: i, op: op})
		}
	}

	// For each file, determine ordering constraints
	for _, ops := range fileOps {
		for i := 1; i < len(ops); i++ {
			curr := ops[i]
			// Look backward for the most recent conflicting operation
			for j := i - 1; j >= 0; j-- {
				prev := ops[j]
				if hasConflict(prev.op, curr.op) {
					stepID := fmt.Sprintf("step_%d", curr.stepIdx)
					depID := fmt.Sprintf("step_%d", prev.stepIdx)
					// Avoid duplicates
					if !planContains(deps[stepID], depID) {
						deps[stepID] = append(deps[stepID], depID)
					}
					break // only depend on the most recent conflicting op
				}
			}
		}
	}

	return deps
}

// GroupParallel groups execution steps that can run simultaneously into batches.
// Steps within the same group have no mutual dependencies and can execute in
// parallel.
func (ep *ExecutionPlanner) GroupParallel(plan *ExecutionPlan) [][]ExecutionStep {
	if len(plan.Steps) == 0 {
		return nil
	}

	// Topological sort using Kahn's algorithm with grouping
	// Steps with no remaining dependencies go into the current group

	// Build in-degree map and adjacency list
	inDegree := make(map[string]int)
	dependents := make(map[string][]string) // step -> steps that depend on it

	stepMap := make(map[string]ExecutionStep)
	for _, step := range plan.Steps {
		stepMap[step.ID] = step
		inDegree[step.ID] = 0
	}

	for stepID, depList := range plan.Dependencies {
		if _, exists := stepMap[stepID]; !exists {
			continue
		}
		inDegree[stepID] = len(depList)
		for _, depID := range depList {
			dependents[depID] = append(dependents[depID], stepID)
		}
	}

	var groups [][]ExecutionStep
	processed := make(map[string]bool)

	for len(processed) < len(plan.Steps) {
		// Find all steps with zero in-degree that haven't been processed
		var currentGroup []ExecutionStep
		for _, step := range plan.Steps {
			if !processed[step.ID] && inDegree[step.ID] == 0 {
				currentGroup = append(currentGroup, step)
			}
		}

		if len(currentGroup) == 0 {
			// Cycle detected or all processed; break to avoid infinite loop
			// Add remaining steps as a final sequential group
			for _, step := range plan.Steps {
				if !processed[step.ID] {
					currentGroup = append(currentGroup, step)
				}
			}
			if len(currentGroup) > 0 {
				groups = append(groups, currentGroup)
			}
			break
		}

		// Sort within group by priority for deterministic ordering
		sort.Slice(currentGroup, func(i, j int) bool {
			return currentGroup[i].Priority < currentGroup[j].Priority
		})

		groups = append(groups, currentGroup)

		// Mark processed and reduce in-degrees
		for _, step := range currentGroup {
			processed[step.ID] = true
			for _, depID := range dependents[step.ID] {
				inDegree[depID]--
			}
		}
	}

	return groups
}

// EstimateDuration calculates the total estimated execution time for a plan,
// accounting for parallelism within groups. The total time is the sum of the
// maximum duration within each group.
func (ep *ExecutionPlanner) EstimateDuration(plan *ExecutionPlan) time.Duration {
	groups := ep.GroupParallel(plan)

	var total time.Duration
	for _, group := range groups {
		// Within a parallel group, total time is the max of the individual steps
		var maxDuration time.Duration
		for _, step := range group {
			if step.EstimatedDuration > maxDuration {
				maxDuration = step.EstimatedDuration
			}
		}
		total += maxDuration
	}

	return total
}

// RecordTiming updates the timing estimate for a tool based on actual observed
// duration. Uses exponential moving average to smooth estimates.
func (ep *ExecutionPlanner) RecordTiming(tool string, duration time.Duration) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	existing, ok := ep.ToolTimings[tool]
	if !ok {
		ep.ToolTimings[tool] = duration
		return
	}

	// Exponential moving average with alpha=0.3
	alpha := 0.3
	newEstimate := time.Duration(float64(existing)*(1-alpha) + float64(duration)*alpha)
	ep.ToolTimings[tool] = newEstimate
}

// FormatPlan produces a human-readable representation of the execution plan
// showing groups, parallelism, and timing estimates.
func (ep *ExecutionPlanner) FormatPlan(plan *ExecutionPlan) string {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if len(plan.Steps) == 0 {
		return "Execution Plan (empty)\n"
	}

	groups := ep.GroupParallel(plan)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Execution Plan (est. %s):\n", planFormatDuration(plan.TotalEstimatedTime)))
	sb.WriteString("─────────────────────────────\n")

	for i, group := range groups {
		groupType := "sequential"
		if len(group) > 1 {
			groupType = "parallel"
		}
		sb.WriteString(fmt.Sprintf("Group %d (%s):\n", i+1, groupType))

		for _, step := range group {
			target := planFormatTarget(step.Args)
			sb.WriteString(fmt.Sprintf("  %s(%s)%s~%s\n",
				step.ToolName,
				target,
				planPadding(step.ToolName, target),
				planFormatDuration(step.EstimatedDuration),
			))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Optimize reorders plan steps to minimize total execution time by maximizing
// parallelism. It reassigns priorities and groups to achieve the optimal schedule.
func (ep *ExecutionPlanner) Optimize(plan *ExecutionPlan) *ExecutionPlan {
	ep.mu.RLock()
	defer ep.mu.RUnlock()

	if len(plan.Steps) == 0 {
		return plan
	}

	// Recompute groups with optimized ordering
	// Strategy: within dependency constraints, prioritize longer-running tasks
	// earlier to maximize overlap with shorter tasks

	// Calculate critical path for each step
	criticalPath := make(map[string]time.Duration)
	stepMap := make(map[string]*ExecutionStep)
	for i := range plan.Steps {
		stepMap[plan.Steps[i].ID] = &plan.Steps[i]
	}

	// Compute longest path from each node to any leaf
	var computeCritical func(stepID string) time.Duration
	computeCritical = func(stepID string) time.Duration {
		if cached, ok := criticalPath[stepID]; ok {
			return cached
		}
		step := stepMap[stepID]
		if step == nil {
			return 0
		}
		maxDownstream := time.Duration(0)
		// Find steps that depend on this one
		for _, s := range plan.Steps {
			for _, dep := range plan.Dependencies[s.ID] {
				if dep == stepID {
					downstream := computeCritical(s.ID)
					if downstream > maxDownstream {
						maxDownstream = downstream
					}
				}
			}
		}
		criticalPath[stepID] = step.EstimatedDuration + maxDownstream
		return criticalPath[stepID]
	}

	for _, step := range plan.Steps {
		computeCritical(step.ID)
	}

	// Re-assign priorities based on critical path (longer critical path = higher priority)
	type stepPriority struct {
		idx      int
		critical time.Duration
	}
	priorities := make([]stepPriority, len(plan.Steps))
	for i, step := range plan.Steps {
		priorities[i] = stepPriority{idx: i, critical: criticalPath[step.ID]}
	}
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i].critical > priorities[j].critical
	})

	// Create optimized plan
	optimized := &ExecutionPlan{
		Steps:        make([]ExecutionStep, len(plan.Steps)),
		Dependencies: plan.Dependencies,
	}
	copy(optimized.Steps, plan.Steps)

	// Assign new priorities
	for rank, sp := range priorities {
		optimized.Steps[sp.idx].Priority = rank
	}

	// Regroup with new priorities
	groups := ep.GroupParallel(optimized)
	for groupIdx, group := range groups {
		for _, step := range group {
			for i := range optimized.Steps {
				if optimized.Steps[i].ID == step.ID {
					optimized.Steps[i].Group = groupIdx + 1
					optimized.Steps[i].CanParallel = len(group) > 1
				}
			}
		}
	}

	optimized.Parallelizable = false
	for _, group := range groups {
		if len(group) > 1 {
			optimized.Parallelizable = true
			break
		}
	}

	optimized.TotalEstimatedTime = ep.EstimateDuration(optimized)

	return optimized
}

// estimateToolDuration returns the current timing estimate for a given tool.
func (ep *ExecutionPlanner) estimateToolDuration(toolName string) time.Duration {
	if d, ok := ep.ToolTimings[toolName]; ok {
		return d
	}
	// Default for unknown tools
	return 1 * time.Second
}

// classifyOp determines the operation type (read/write/edit) for a tool.
func classifyOp(toolName string) string {
	switch toolName {
	case "Read", "Grep", "Glob":
		return "read"
	case "Write":
		return "write"
	case "Edit":
		return "edit"
	case "Bash":
		return "write" // conservative: bash may modify files
	case "WebFetch":
		return "read"
	default:
		return "write"
	}
}

// hasConflict determines if two operations on the same file create a dependency.
func hasConflict(prevOp, currOp string) bool {
	// Read after read: no conflict (parallel OK)
	if prevOp == "read" && currOp == "read" {
		return false
	}
	// Write/edit before read: conflict (must wait for write to complete)
	// Write/edit before write/edit: conflict (must be sequential)
	// Read before write/edit: conflict (must read before modification)
	return true
}

// planContains checks if a slice contains a given string.
func planContains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// planFormatDuration produces a human-friendly duration string.
func planFormatDuration(d time.Duration) string {
	if d >= time.Second {
		secs := float64(d) / float64(time.Second)
		if secs == float64(int(secs)) {
			return fmt.Sprintf("%ds", int(secs))
		}
		return fmt.Sprintf("%.1fs", secs)
	}
	return fmt.Sprintf("%dms", d.Milliseconds())
}

// planFormatTarget extracts a display-friendly target from step args.
func planFormatTarget(args map[string]interface{}) string {
	if args == nil {
		return ""
	}
	// Try common arg names
	for _, key := range []string{"file_path", "path", "pattern", "command", "query", "url"} {
		if v, ok := args[key]; ok {
			s := fmt.Sprintf("%v", v)
			// Truncate long values
			if len(s) > 30 {
				s = s[:27] + "..."
			}
			return s
		}
	}
	return ""
}

// planPadding generates alignment spacing for formatted output.
func planPadding(toolName, target string) string {
	totalLen := len(toolName) + len(target) + 2 // parens
	padNeeded := 30 - totalLen
	if padNeeded < 2 {
		padNeeded = 2
	}
	return strings.Repeat(" ", padNeeded)
}
