package engine

import (
	"strings"
	"testing"
	"time"
)

func TestNewExecutionPlanner(t *testing.T) {
	ep := NewExecutionPlanner()
	if ep == nil {
		t.Fatal("NewExecutionPlanner returned nil")
	}
	if len(ep.ToolTimings) != 7 {
		t.Errorf("expected 7 default timings, got %d", len(ep.ToolTimings))
	}

	expected := map[string]time.Duration{
		"Read":     100 * time.Millisecond,
		"Edit":     200 * time.Millisecond,
		"Write":    200 * time.Millisecond,
		"Bash":     5 * time.Second,
		"Grep":     500 * time.Millisecond,
		"Glob":     200 * time.Millisecond,
		"WebFetch": 3 * time.Second,
	}
	for tool, dur := range expected {
		if ep.ToolTimings[tool] != dur {
			t.Errorf("timing for %s: got %v, want %v", tool, ep.ToolTimings[tool], dur)
		}
	}
}

func TestPlan_EmptyCalls(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := ep.Plan(nil)
	if plan == nil {
		t.Fatal("Plan returned nil for empty input")
	}
	if len(plan.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(plan.Steps))
	}
	if plan.Parallelizable {
		t.Error("empty plan should not be parallelizable")
	}
}

func TestPlan_IndependentReads(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/handler.go"}, Targets: []string{"src/handler.go"}},
		{ToolName: "Grep", Args: map[string]interface{}{"pattern": "ValidateToken"}, Targets: []string{}},
	}

	plan := ep.Plan(calls)

	if !plan.Parallelizable {
		t.Error("independent reads should be parallelizable")
	}
	if len(plan.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(plan.Steps))
	}

	// All steps should be in group 1 (no dependencies between them)
	for _, step := range plan.Steps {
		if step.Group != 1 {
			t.Errorf("step %s should be in group 1, got group %d", step.ID, step.Group)
		}
		if !step.CanParallel {
			t.Errorf("step %s should be marked as CanParallel", step.ID)
		}
	}
}

func TestPlan_WriteBeforeRead(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Write", Args: map[string]interface{}{"file_path": "src/new.go"}, Targets: []string{"src/new.go"}},
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/new.go"}, Targets: []string{"src/new.go"}},
	}

	plan := ep.Plan(calls)

	if plan.Parallelizable {
		t.Error("write-then-read to same file should not be parallelizable")
	}

	// step_1 should depend on step_0
	if deps, ok := plan.Dependencies["step_1"]; !ok || len(deps) == 0 {
		t.Error("step_1 should depend on step_0")
	} else if deps[0] != "step_0" {
		t.Errorf("step_1 should depend on step_0, got %v", deps)
	}

	// Steps should be in different groups
	if plan.Steps[0].Group == plan.Steps[1].Group {
		t.Error("dependent steps should be in different groups")
	}
}

func TestPlan_EditSameFileSequential(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Edit", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
		{ToolName: "Edit", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
	}

	plan := ep.Plan(calls)

	if plan.Parallelizable {
		t.Error("edits to same file should be sequential")
	}
	if _, ok := plan.Dependencies["step_1"]; !ok {
		t.Error("second edit should depend on first edit")
	}
}

func TestPlan_MixedDependencies(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/handler.go"}, Targets: []string{"src/handler.go"}},
		{ToolName: "Edit", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
		{ToolName: "Edit", Args: map[string]interface{}{"file_path": "src/handler.go"}, Targets: []string{"src/handler.go"}},
		{ToolName: "Bash", Args: map[string]interface{}{"command": "go test ./..."}, Targets: []string{"src/auth.go", "src/handler.go"}},
	}

	plan := ep.Plan(calls)

	// Reads should be in group 1
	if plan.Steps[0].Group != 1 || plan.Steps[1].Group != 1 {
		t.Error("reads should be in the first group")
	}

	// Edits depend on their respective reads
	if plan.Steps[2].Group <= plan.Steps[0].Group {
		t.Error("edit of auth.go should be after read of auth.go")
	}
	if plan.Steps[3].Group <= plan.Steps[1].Group {
		t.Error("edit of handler.go should be after read of handler.go")
	}

	// Bash depends on both edits
	bashStep := plan.Steps[4]
	if bashStep.Group <= plan.Steps[2].Group || bashStep.Group <= plan.Steps[3].Group {
		t.Error("bash should be after both edits")
	}
}

func TestFindDependencies_ReadReadNoDep(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Read", Targets: []string{"file.go"}},
		{ToolName: "Read", Targets: []string{"file.go"}},
	}

	deps := ep.FindDependencies(calls)
	if len(deps) != 0 {
		t.Errorf("read-read on same file should have no dependencies, got %v", deps)
	}
}

func TestFindDependencies_WriteRead(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Write", Targets: []string{"file.go"}},
		{ToolName: "Read", Targets: []string{"file.go"}},
	}

	deps := ep.FindDependencies(calls)
	if depList, ok := deps["step_1"]; !ok {
		t.Error("read after write should create dependency")
	} else if len(depList) != 1 || depList[0] != "step_0" {
		t.Errorf("unexpected dependencies: %v", depList)
	}
}

func TestFindDependencies_GrepGlobParallel(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Grep", Targets: []string{}},
		{ToolName: "Glob", Targets: []string{}},
	}

	deps := ep.FindDependencies(calls)
	if len(deps) != 0 {
		t.Errorf("grep and glob with no shared targets should have no dependencies, got %v", deps)
	}
}

func TestGroupParallel_AllIndependent(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{ID: "step_0", ToolName: "Read", Priority: 0, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_1", ToolName: "Read", Priority: 1, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_2", ToolName: "Grep", Priority: 2, EstimatedDuration: 500 * time.Millisecond},
		},
		Dependencies: map[string][]string{},
	}

	groups := ep.GroupParallel(plan)
	if len(groups) != 1 {
		t.Errorf("all independent steps should be in 1 group, got %d groups", len(groups))
	}
	if len(groups[0]) != 3 {
		t.Errorf("group should have 3 steps, got %d", len(groups[0]))
	}
}

func TestGroupParallel_LinearChain(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{ID: "step_0", ToolName: "Read", Priority: 0, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_1", ToolName: "Edit", Priority: 1, EstimatedDuration: 200 * time.Millisecond},
			{ID: "step_2", ToolName: "Bash", Priority: 2, EstimatedDuration: 5 * time.Second},
		},
		Dependencies: map[string][]string{
			"step_1": {"step_0"},
			"step_2": {"step_1"},
		},
	}

	groups := ep.GroupParallel(plan)
	if len(groups) != 3 {
		t.Errorf("linear chain should produce 3 groups, got %d", len(groups))
	}
	for i, group := range groups {
		if len(group) != 1 {
			t.Errorf("group %d should have 1 step, got %d", i, len(group))
		}
	}
}

func TestGroupParallel_DiamondDeps(t *testing.T) {
	ep := NewExecutionPlanner()
	// Diamond: step_0 -> step_1, step_0 -> step_2, step_1 -> step_3, step_2 -> step_3
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{ID: "step_0", ToolName: "Read", Priority: 0, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_1", ToolName: "Edit", Priority: 1, EstimatedDuration: 200 * time.Millisecond},
			{ID: "step_2", ToolName: "Edit", Priority: 2, EstimatedDuration: 200 * time.Millisecond},
			{ID: "step_3", ToolName: "Bash", Priority: 3, EstimatedDuration: 5 * time.Second},
		},
		Dependencies: map[string][]string{
			"step_1": {"step_0"},
			"step_2": {"step_0"},
			"step_3": {"step_1", "step_2"},
		},
	}

	groups := ep.GroupParallel(plan)
	if len(groups) != 3 {
		t.Errorf("diamond should produce 3 groups, got %d", len(groups))
	}
	// Group 1: step_0
	if len(groups[0]) != 1 || groups[0][0].ID != "step_0" {
		t.Errorf("group 1 should be [step_0], got %v", groups[0])
	}
	// Group 2: step_1, step_2 (parallel)
	if len(groups[1]) != 2 {
		t.Errorf("group 2 should have 2 steps, got %d", len(groups[1]))
	}
	// Group 3: step_3
	if len(groups[2]) != 1 || groups[2][0].ID != "step_3" {
		t.Errorf("group 3 should be [step_3], got %v", groups[2])
	}
}

func TestEstimateDuration_ParallelGroup(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{ID: "step_0", ToolName: "Read", Priority: 0, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_1", ToolName: "Grep", Priority: 1, EstimatedDuration: 500 * time.Millisecond},
			{ID: "step_2", ToolName: "Edit", Priority: 2, EstimatedDuration: 200 * time.Millisecond},
		},
		Dependencies: map[string][]string{
			"step_2": {"step_0", "step_1"},
		},
	}

	duration := ep.EstimateDuration(plan)
	// Group 1 (parallel): max(100ms, 500ms) = 500ms
	// Group 2: 200ms
	// Total: 700ms
	expected := 700 * time.Millisecond
	if duration != expected {
		t.Errorf("estimated duration: got %v, want %v", duration, expected)
	}
}

func TestEstimateDuration_Sequential(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{ID: "step_0", ToolName: "Read", Priority: 0, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_1", ToolName: "Edit", Priority: 1, EstimatedDuration: 200 * time.Millisecond},
			{ID: "step_2", ToolName: "Bash", Priority: 2, EstimatedDuration: 5 * time.Second},
		},
		Dependencies: map[string][]string{
			"step_1": {"step_0"},
			"step_2": {"step_1"},
		},
	}

	duration := ep.EstimateDuration(plan)
	expected := 100*time.Millisecond + 200*time.Millisecond + 5*time.Second
	if duration != expected {
		t.Errorf("estimated duration: got %v, want %v", duration, expected)
	}
}

func TestRecordTiming(t *testing.T) {
	ep := NewExecutionPlanner()

	// Initial timing for Read is 100ms
	if ep.ToolTimings["Read"] != 100*time.Millisecond {
		t.Fatal("unexpected initial Read timing")
	}

	// Record a slower actual timing
	ep.RecordTiming("Read", 200*time.Millisecond)

	// Should be EMA: 100*0.7 + 200*0.3 = 70 + 60 = 130ms
	expected := 130 * time.Millisecond
	if ep.ToolTimings["Read"] != expected {
		t.Errorf("after RecordTiming: got %v, want %v", ep.ToolTimings["Read"], expected)
	}

	// Record timing for unknown tool
	ep.RecordTiming("CustomTool", 2*time.Second)
	if ep.ToolTimings["CustomTool"] != 2*time.Second {
		t.Errorf("new tool timing: got %v, want %v", ep.ToolTimings["CustomTool"], 2*time.Second)
	}
}

func TestExecutionPlannerFormatPlan(t *testing.T) {
	ep := NewExecutionPlanner()
	calls := []PlannedCall{
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/handler.go"}, Targets: []string{"src/handler.go"}},
		{ToolName: "Edit", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
	}

	plan := ep.Plan(calls)
	output := ep.FormatPlan(plan)

	if !strings.Contains(output, "Execution Plan") {
		t.Error("format should contain 'Execution Plan' header")
	}
	if !strings.Contains(output, "Group 1") {
		t.Error("format should contain 'Group 1'")
	}
	if !strings.Contains(output, "parallel") {
		t.Error("format should indicate parallel groups")
	}
	if !strings.Contains(output, "Read") {
		t.Error("format should contain tool names")
	}
	if !strings.Contains(output, "src/auth.go") {
		t.Error("format should contain file paths")
	}
}

func TestExecutionPlannerFormatPlan_Empty(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := ep.Plan(nil)
	output := ep.FormatPlan(plan)
	if !strings.Contains(output, "empty") {
		t.Error("empty plan should be indicated in format output")
	}
}

func TestOptimize(t *testing.T) {
	ep := NewExecutionPlanner()

	// Create a plan where optimization can reorder for better parallelism
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{ID: "step_0", ToolName: "Read", Priority: 0, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_1", ToolName: "Bash", Priority: 1, EstimatedDuration: 5 * time.Second},
			{ID: "step_2", ToolName: "Read", Priority: 2, EstimatedDuration: 100 * time.Millisecond},
		},
		Dependencies: map[string][]string{},
	}

	optimized := ep.Optimize(plan)

	if optimized == nil {
		t.Fatal("Optimize returned nil")
	}
	if len(optimized.Steps) != 3 {
		t.Errorf("optimized plan should have 3 steps, got %d", len(optimized.Steps))
	}
	if !optimized.Parallelizable {
		t.Error("optimized plan with independent steps should be parallelizable")
	}

	// The Bash step should have highest priority (longest critical path)
	for _, step := range optimized.Steps {
		if step.ID == "step_1" && step.Priority != 0 {
			t.Errorf("Bash step should have priority 0 (highest), got %d", step.Priority)
		}
	}
}

func TestOptimize_WithDeps(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := &ExecutionPlan{
		Steps: []ExecutionStep{
			{ID: "step_0", ToolName: "Read", Priority: 0, EstimatedDuration: 100 * time.Millisecond},
			{ID: "step_1", ToolName: "Edit", Priority: 1, EstimatedDuration: 200 * time.Millisecond},
			{ID: "step_2", ToolName: "Bash", Priority: 2, EstimatedDuration: 5 * time.Second},
		},
		Dependencies: map[string][]string{
			"step_1": {"step_0"},
			"step_2": {"step_1"},
		},
	}

	optimized := ep.Optimize(plan)

	// Should preserve dependency ordering even after optimization
	if optimized.Steps[0].Group >= optimized.Steps[1].Group {
		// Verify through groups that ordering is maintained
		groupMap := make(map[string]int)
		for _, step := range optimized.Steps {
			groupMap[step.ID] = step.Group
		}
		if groupMap["step_0"] >= groupMap["step_1"] {
			t.Error("step_0 should be in an earlier group than step_1")
		}
		if groupMap["step_1"] >= groupMap["step_2"] {
			t.Error("step_1 should be in an earlier group than step_2")
		}
	}

	// Total time should remain the sum since it's all sequential
	expectedTime := 100*time.Millisecond + 200*time.Millisecond + 5*time.Second
	if optimized.TotalEstimatedTime != expectedTime {
		t.Errorf("optimized time: got %v, want %v", optimized.TotalEstimatedTime, expectedTime)
	}
}

func TestOptimize_Empty(t *testing.T) {
	ep := NewExecutionPlanner()
	plan := &ExecutionPlan{
		Steps:        nil,
		Dependencies: map[string][]string{},
	}

	optimized := ep.Optimize(plan)
	if len(optimized.Steps) != 0 {
		t.Error("optimizing empty plan should return empty plan")
	}
}

func TestPlan_FullScenario(t *testing.T) {
	ep := NewExecutionPlanner()

	// Simulate a realistic multi-step task
	calls := []PlannedCall{
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
		{ToolName: "Read", Args: map[string]interface{}{"file_path": "src/handler.go"}, Targets: []string{"src/handler.go"}},
		{ToolName: "Grep", Args: map[string]interface{}{"pattern": "ValidateToken"}, Targets: []string{}},
		{ToolName: "Edit", Args: map[string]interface{}{"file_path": "src/auth.go"}, Targets: []string{"src/auth.go"}},
		{ToolName: "Edit", Args: map[string]interface{}{"file_path": "src/handler.go"}, Targets: []string{"src/handler.go"}},
		{ToolName: "Write", Args: map[string]interface{}{"file_path": "src/middleware.go"}, Targets: []string{"src/middleware.go"}},
		{ToolName: "Bash", Args: map[string]interface{}{"command": "go test ./..."}, Targets: []string{"src/auth.go", "src/handler.go", "src/middleware.go"}},
	}

	plan := ep.Plan(calls)

	// Verify structure
	if len(plan.Steps) != 7 {
		t.Fatalf("expected 7 steps, got %d", len(plan.Steps))
	}
	if !plan.Parallelizable {
		t.Error("plan should be parallelizable")
	}

	// Reads and grep should be in the first group (parallel)
	if plan.Steps[0].Group != 1 || plan.Steps[1].Group != 1 || plan.Steps[2].Group != 1 {
		t.Errorf("reads/grep groups: %d, %d, %d (all should be 1)",
			plan.Steps[0].Group, plan.Steps[1].Group, plan.Steps[2].Group)
	}

	// Edits should be after their respective reads
	if plan.Steps[3].Group <= plan.Steps[0].Group {
		t.Error("edit of auth.go should come after read of auth.go")
	}
	if plan.Steps[4].Group <= plan.Steps[1].Group {
		t.Error("edit of handler.go should come after read of handler.go")
	}

	// Bash should be last
	bashGroup := plan.Steps[6].Group
	for i := 0; i < 6; i++ {
		if plan.Steps[i].Group >= bashGroup {
			t.Errorf("step %d (group %d) should be before bash (group %d)", i, plan.Steps[i].Group, bashGroup)
		}
	}

	// Total estimated time should account for parallelism
	// Sequential would be: 100+100+500+200+200+200+5000 = 6300ms
	// With parallelism should be less
	sequentialTotal := 100 + 100 + 500 + 200 + 200 + 200 + 5000
	if plan.TotalEstimatedTime >= time.Duration(sequentialTotal)*time.Millisecond {
		t.Errorf("parallel plan (%v) should be faster than sequential (%dms)",
			plan.TotalEstimatedTime, sequentialTotal)
	}
}

func TestRecordTiming_ConcurrentSafe(t *testing.T) {
	ep := NewExecutionPlanner()

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			ep.RecordTiming("Read", 150*time.Millisecond)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	// Just verify no race/panic occurred
	if ep.ToolTimings["Read"] == 0 {
		t.Error("timing should be non-zero after recording")
	}
}

func TestClassifyOp(t *testing.T) {
	tests := []struct {
		tool     string
		expected string
	}{
		{"Read", "read"},
		{"Grep", "read"},
		{"Glob", "read"},
		{"Write", "write"},
		{"Edit", "edit"},
		{"Bash", "write"},
		{"WebFetch", "read"},
		{"Unknown", "write"},
	}

	for _, tt := range tests {
		got := classifyOp(tt.tool)
		if got != tt.expected {
			t.Errorf("classifyOp(%s) = %s, want %s", tt.tool, got, tt.expected)
		}
	}
}

func TestHasConflict(t *testing.T) {
	tests := []struct {
		prev     string
		curr     string
		conflict bool
	}{
		{"read", "read", false},
		{"write", "read", true},
		{"edit", "read", true},
		{"read", "write", true},
		{"read", "edit", true},
		{"write", "write", true},
		{"edit", "edit", true},
		{"write", "edit", true},
	}

	for _, tt := range tests {
		got := hasConflict(tt.prev, tt.curr)
		if got != tt.conflict {
			t.Errorf("hasConflict(%s, %s) = %v, want %v", tt.prev, tt.curr, got, tt.conflict)
		}
	}
}

func TestPlanFormatDuration(t *testing.T) {
	tests := []struct {
		dur      time.Duration
		expected string
	}{
		{100 * time.Millisecond, "100ms"},
		{500 * time.Millisecond, "500ms"},
		{1 * time.Second, "1s"},
		{5 * time.Second, "5s"},
		{2300 * time.Millisecond, "2.3s"},
		{1500 * time.Millisecond, "1.5s"},
	}

	for _, tt := range tests {
		got := planFormatDuration(tt.dur)
		if got != tt.expected {
			t.Errorf("formatDuration(%v) = %s, want %s", tt.dur, got, tt.expected)
		}
	}
}

func TestPlanContains(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !planContains(slice, "b") {
		t.Error("should find 'b' in slice")
	}
	if planContains(slice, "d") {
		t.Error("should not find 'd' in slice")
	}
	if planContains(nil, "a") {
		t.Error("nil slice should not contain anything")
	}
}

func TestPlanFormatTarget(t *testing.T) {
	tests := []struct {
		args     map[string]interface{}
		expected string
	}{
		{map[string]interface{}{"file_path": "src/main.go"}, "src/main.go"},
		{map[string]interface{}{"pattern": "TODO"}, "TODO"},
		{map[string]interface{}{"command": "go test"}, "go test"},
		{nil, ""},
		{map[string]interface{}{"unknown_key": "val"}, ""},
	}

	for _, tt := range tests {
		got := planFormatTarget(tt.args)
		if got != tt.expected {
			t.Errorf("formatTarget(%v) = %q, want %q", tt.args, got, tt.expected)
		}
	}
}

func TestPlanFormatTarget_Truncation(t *testing.T) {
	args := map[string]interface{}{
		"file_path": "this/is/a/very/long/file/path/that/exceeds/thirty/characters.go",
	}
	result := planFormatTarget(args)
	if len(result) > 30 {
		t.Errorf("formatTarget should truncate long values, got length %d", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated values should end with ...")
	}
}
