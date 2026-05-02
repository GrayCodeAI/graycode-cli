package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockMemory struct {
	guidelines []string
	learned    []struct{ pattern, lesson string }
	learnErr   error
}

func (m *mockMemory) Learn(pattern, lesson string) error {
	if m.learnErr != nil {
		return m.learnErr
	}
	m.learned = append(m.learned, struct{ pattern, lesson string }{pattern, lesson})
	return nil
}

func (m *mockMemory) Retrieve(query string) []string {
	return m.guidelines
}

func (m *mockMemory) Format() string {
	if len(m.guidelines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, g := range m.guidelines {
		b.WriteString("- " + g + "\n")
	}
	return b.String()
}

type mockSkillStore struct {
	skills    []string
	distilled []struct{ goal, outcome string; steps []string }
	distillErr error
}

func (m *mockSkillStore) Distill(goal string, steps []string, outcome string) error {
	if m.distillErr != nil {
		return m.distillErr
	}
	m.distilled = append(m.distilled, struct{ goal, outcome string; steps []string }{goal, outcome, steps})
	return nil
}

func (m *mockSkillStore) Retrieve(query string) []string {
	return m.skills
}

type mockCostTracker struct {
	entries  []CostEntry
	total    float64
	recordErr error
}

func (m *mockCostTracker) Record(entry CostEntry) error {
	if m.recordErr != nil {
		return m.recordErr
	}
	m.entries = append(m.entries, entry)
	m.total += entry.TotalCost
	return nil
}

func (m *mockCostTracker) SessionTotal() float64 {
	return m.total
}

// --- OnSessionStart tests ---

func TestOnSessionStart_RetrievesGuidelines(t *testing.T) {
	mem := &mockMemory{
		guidelines: []string{
			"When editing Go files, always run tests after",
			"Use Bash for file discovery before editing",
		},
	}
	lc := &SessionLifecycle{Memory: mem}

	result := lc.OnSessionStart(context.Background(), "fix the bug in main.go")

	if !strings.Contains(result, "## Learned Guidelines") {
		t.Error("expected Learned Guidelines header")
	}
	if !strings.Contains(result, "When editing Go files") {
		t.Error("expected first guideline in output")
	}
	if !strings.Contains(result, "Use Bash for file discovery") {
		t.Error("expected second guideline in output")
	}
}

func TestOnSessionStart_RetrievesSkills(t *testing.T) {
	skills := &mockSkillStore{
		skills: []string{
			"Go test debugging: run with -v flag, check stderr",
		},
	}
	lc := &SessionLifecycle{SkillStore: skills}

	result := lc.OnSessionStart(context.Background(), "debug test failure")

	if !strings.Contains(result, "## Relevant Skills") {
		t.Error("expected Relevant Skills header")
	}
	if !strings.Contains(result, "Go test debugging") {
		t.Error("expected skill in output")
	}
}

func TestOnSessionStart_CombinesGuidelinesAndSkills(t *testing.T) {
	mem := &mockMemory{guidelines: []string{"guideline one"}}
	skills := &mockSkillStore{skills: []string{"skill one"}}
	lc := &SessionLifecycle{Memory: mem, SkillStore: skills}

	result := lc.OnSessionStart(context.Background(), "do something")

	if !strings.Contains(result, "## Learned Guidelines") {
		t.Error("expected guidelines section")
	}
	if !strings.Contains(result, "## Relevant Skills") {
		t.Error("expected skills section")
	}
	// Guidelines should appear before skills.
	gIdx := strings.Index(result, "## Learned Guidelines")
	sIdx := strings.Index(result, "## Relevant Skills")
	if gIdx >= sIdx {
		t.Error("expected guidelines before skills")
	}
}

func TestOnSessionStart_EmptyWhenNothingRetrieved(t *testing.T) {
	mem := &mockMemory{guidelines: nil}
	skills := &mockSkillStore{skills: nil}
	lc := &SessionLifecycle{Memory: mem, SkillStore: skills}

	result := lc.OnSessionStart(context.Background(), "hello")

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestOnSessionStart_NilDependencies(t *testing.T) {
	lc := &SessionLifecycle{}

	result := lc.OnSessionStart(context.Background(), "test")

	if result != "" {
		t.Errorf("expected empty string with nil dependencies, got %q", result)
	}
}

// --- OnSessionEnd tests ---

func TestOnSessionEnd_SuccessfulSession_LearnsGuideline(t *testing.T) {
	mem := &mockMemory{}
	lc := &SessionLifecycle{Memory: mem}

	outcome := SessionOutcome{
		Success:      true,
		TaskGoal:     "fix authentication bug",
		ToolsUsed:    []string{"Bash", "Edit"},
		FilesChanged: []string{"auth.go"},
		TotalCost:    0.05,
		Duration:     2 * time.Minute,
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mem.learned) == 0 {
		t.Fatal("expected a guideline to be learned")
	}
	learned := mem.learned[0]
	if !strings.Contains(learned.pattern, "fix authentication bug") {
		t.Errorf("expected pattern to mention task goal, got %q", learned.pattern)
	}
	if !strings.Contains(learned.lesson, "Bash") || !strings.Contains(learned.lesson, "Edit") {
		t.Errorf("expected lesson to mention tools used, got %q", learned.lesson)
	}
	if !strings.Contains(learned.lesson, "auth.go") {
		t.Errorf("expected lesson to mention files changed, got %q", learned.lesson)
	}
}

func TestOnSessionEnd_FailedSession_LearnsWarning(t *testing.T) {
	mem := &mockMemory{}
	lc := &SessionLifecycle{Memory: mem}

	outcome := SessionOutcome{
		Success:   false,
		TaskGoal:  "deploy to production",
		ToolsUsed: []string{"Bash"},
		TotalCost: 0.10,
		Duration:  5 * time.Minute,
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mem.learned) == 0 {
		t.Fatal("expected a warning guideline to be learned")
	}
	learned := mem.learned[0]
	if !strings.Contains(learned.lesson, "did not succeed") {
		t.Errorf("expected failure lesson, got %q", learned.lesson)
	}
	if !strings.Contains(learned.lesson, "alternative strategies") {
		t.Errorf("expected alternative strategies suggestion, got %q", learned.lesson)
	}
}

func TestOnSessionEnd_ComplexSuccess_DistillsSkill(t *testing.T) {
	skills := &mockSkillStore{}
	lc := &SessionLifecycle{SkillStore: skills}

	outcome := SessionOutcome{
		Success:      true,
		TaskGoal:     "refactor database layer",
		FilesChanged: []string{"db.go", "models.go"},
		ToolsUsed:    []string{"Bash", "Edit", "Read"},
		TotalCost:    0.20,
		Duration:     10 * time.Minute,
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills.distilled) == 0 {
		t.Fatal("expected skill distillation for complex successful task")
	}
	d := skills.distilled[0]
	if d.goal != "refactor database layer" {
		t.Errorf("expected goal 'refactor database layer', got %q", d.goal)
	}
	if d.outcome != "success" {
		t.Errorf("expected outcome 'success', got %q", d.outcome)
	}
}

func TestOnSessionEnd_SimpleSuccess_NoSkillDistillation(t *testing.T) {
	skills := &mockSkillStore{}
	lc := &SessionLifecycle{SkillStore: skills}

	outcome := SessionOutcome{
		Success:      true,
		TaskGoal:     "fix typo",
		FilesChanged: []string{"readme.md"},
		ToolsUsed:    []string{"Edit"},
		TotalCost:    0.01,
		Duration:     30 * time.Second,
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills.distilled) != 0 {
		t.Error("expected no skill distillation for simple task")
	}
}

func TestOnSessionEnd_FailedSession_NoSkillDistillation(t *testing.T) {
	skills := &mockSkillStore{}
	lc := &SessionLifecycle{SkillStore: skills}

	outcome := SessionOutcome{
		Success:      false,
		TaskGoal:     "complex refactor",
		FilesChanged: []string{"a.go", "b.go", "c.go"},
		ToolsUsed:    []string{"Bash", "Edit", "Read"},
		TotalCost:    0.30,
		Duration:     15 * time.Minute,
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skills.distilled) != 0 {
		t.Error("expected no skill distillation for failed task")
	}
}

func TestOnSessionEnd_RecordsCostMetrics(t *testing.T) {
	tracker := &mockCostTracker{}
	lc := &SessionLifecycle{CostTracker: tracker}

	outcome := SessionOutcome{
		Success:   true,
		TaskGoal:  "fix bug",
		TotalCost: 0.042,
		Duration:  3 * time.Minute,
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tracker.entries) != 1 {
		t.Fatalf("expected 1 cost entry, got %d", len(tracker.entries))
	}
	entry := tracker.entries[0]
	if entry.TotalCost != 0.042 {
		t.Errorf("expected cost 0.042, got %f", entry.TotalCost)
	}
	if entry.Duration != 3*time.Minute {
		t.Errorf("expected duration 3m, got %v", entry.Duration)
	}
	if !entry.Success {
		t.Error("expected success=true")
	}
	if entry.TaskGoal != "fix bug" {
		t.Errorf("expected task goal 'fix bug', got %q", entry.TaskGoal)
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestOnSessionEnd_UserFeedback_LearnsExtra(t *testing.T) {
	mem := &mockMemory{}
	lc := &SessionLifecycle{Memory: mem}

	outcome := SessionOutcome{
		Success:      true,
		TaskGoal:     "write tests",
		UserFeedback: "table-driven tests are preferred in this project",
		TotalCost:    0.03,
		Duration:     1 * time.Minute,
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have learned both the success guideline and the feedback.
	if len(mem.learned) < 2 {
		t.Fatalf("expected at least 2 learned items, got %d", len(mem.learned))
	}

	// Find the feedback-based learning.
	var foundFeedback bool
	for _, l := range mem.learned {
		if strings.Contains(l.lesson, "table-driven tests") {
			foundFeedback = true
			break
		}
	}
	if !foundFeedback {
		t.Error("expected user feedback to be learned as a guideline")
	}
}

func TestOnSessionEnd_NilDependencies(t *testing.T) {
	lc := &SessionLifecycle{}

	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "test",
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("expected no error with nil dependencies, got: %v", err)
	}
}

func TestOnSessionEnd_EmptyTaskGoal_NoGuideline(t *testing.T) {
	mem := &mockMemory{}
	lc := &SessionLifecycle{Memory: mem}

	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "",
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mem.learned) != 0 {
		t.Errorf("expected no guidelines for empty task goal, got %d", len(mem.learned))
	}
}

func TestOnSessionEnd_AllComponentsTogether(t *testing.T) {
	mem := &mockMemory{}
	skills := &mockSkillStore{}
	tracker := &mockCostTracker{}
	lc := &SessionLifecycle{Memory: mem, SkillStore: skills, CostTracker: tracker}

	outcome := SessionOutcome{
		Success:      true,
		TaskGoal:     "implement new API endpoint",
		FilesChanged: []string{"handler.go", "routes.go", "handler_test.go"},
		ToolsUsed:    []string{"Bash", "Edit", "Read"},
		TotalCost:    0.15,
		Duration:     8 * time.Minute,
		UserFeedback: "good work",
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Memory should have learned the success guideline + user feedback.
	if len(mem.learned) < 2 {
		t.Errorf("expected at least 2 learned guidelines, got %d", len(mem.learned))
	}

	// Skills should have been distilled (complex + successful).
	if len(skills.distilled) != 1 {
		t.Errorf("expected 1 distilled skill, got %d", len(skills.distilled))
	}

	// Cost should have been recorded.
	if len(tracker.entries) != 1 {
		t.Errorf("expected 1 cost entry, got %d", len(tracker.entries))
	}
}

// --- Error propagation tests ---

func TestOnSessionEnd_MemoryLearnError_PropagatesInAggregate(t *testing.T) {
	mem := &mockMemory{learnErr: errors.New("storage full")}
	tracker := &mockCostTracker{}
	lc := &SessionLifecycle{Memory: mem, CostTracker: tracker}

	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "some task",
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err == nil {
		t.Fatal("expected error from memory learn failure")
	}
	if !strings.Contains(err.Error(), "storage full") {
		t.Errorf("expected 'storage full' in error, got: %v", err)
	}

	// Cost should still be recorded even when memory fails.
	if len(tracker.entries) != 1 {
		t.Error("expected cost to still be recorded when memory fails")
	}
}

func TestOnSessionEnd_SkillDistillError_PropagatesInAggregate(t *testing.T) {
	skills := &mockSkillStore{distillErr: errors.New("distill failed")}
	lc := &SessionLifecycle{SkillStore: skills}

	outcome := SessionOutcome{
		Success:      true,
		TaskGoal:     "complex task",
		FilesChanged: []string{"a.go", "b.go"},
		ToolsUsed:    []string{"Bash", "Edit", "Read"},
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err == nil {
		t.Fatal("expected error from skill distill failure")
	}
	if !strings.Contains(err.Error(), "distill failed") {
		t.Errorf("expected 'distill failed' in error, got: %v", err)
	}
}

func TestOnSessionEnd_CostRecordError_PropagatesInAggregate(t *testing.T) {
	tracker := &mockCostTracker{recordErr: errors.New("disk full")}
	lc := &SessionLifecycle{CostTracker: tracker}

	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "some task",
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err == nil {
		t.Fatal("expected error from cost record failure")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("expected 'disk full' in error, got: %v", err)
	}
}

func TestOnSessionEnd_MultipleErrors_AggregatedInMessage(t *testing.T) {
	mem := &mockMemory{learnErr: errors.New("mem fail")}
	tracker := &mockCostTracker{recordErr: errors.New("cost fail")}
	lc := &SessionLifecycle{Memory: mem, CostTracker: tracker}

	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "some task",
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !strings.Contains(err.Error(), "mem fail") {
		t.Errorf("expected 'mem fail' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "cost fail") {
		t.Errorf("expected 'cost fail' in error, got: %v", err)
	}
}

// --- buildGuideline tests ---

func TestBuildGuideline_Success_WithTools(t *testing.T) {
	outcome := SessionOutcome{
		Success:      true,
		TaskGoal:     "add logging",
		ToolsUsed:    []string{"Bash", "Edit"},
		FilesChanged: []string{"logger.go"},
	}

	pattern, lesson := buildGuideline(outcome)

	if !strings.Contains(pattern, "add logging") {
		t.Errorf("expected pattern to contain task goal, got %q", pattern)
	}
	if !strings.Contains(lesson, "Bash, Edit") {
		t.Errorf("expected lesson to mention tools, got %q", lesson)
	}
	if !strings.Contains(lesson, "works well") {
		t.Errorf("expected positive lesson, got %q", lesson)
	}
}

func TestBuildGuideline_Success_NoTools(t *testing.T) {
	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "answer question",
	}

	_, lesson := buildGuideline(outcome)

	if !strings.Contains(lesson, "succeeded") {
		t.Errorf("expected generic success lesson, got %q", lesson)
	}
}

func TestBuildGuideline_Failure(t *testing.T) {
	outcome := SessionOutcome{
		Success:   false,
		TaskGoal:  "deploy",
		ToolsUsed: []string{"Bash"},
	}

	pattern, lesson := buildGuideline(outcome)

	if !strings.Contains(pattern, "deploy") {
		t.Errorf("expected pattern to contain task goal, got %q", pattern)
	}
	if !strings.Contains(lesson, "did not succeed") {
		t.Errorf("expected failure lesson, got %q", lesson)
	}
}

func TestBuildGuideline_EmptyTaskGoal(t *testing.T) {
	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "",
	}

	pattern, lesson := buildGuideline(outcome)

	if pattern != "" || lesson != "" {
		t.Errorf("expected empty guideline for empty task goal, got pattern=%q lesson=%q", pattern, lesson)
	}
}

// --- isComplex tests ---

func TestIsComplex_AboveThreshold(t *testing.T) {
	outcome := SessionOutcome{
		FilesChanged: []string{"a.go", "b.go"},
		ToolsUsed:    []string{"Bash"},
	}
	if !isComplex(outcome) {
		t.Error("expected complex for 2 files + 1 tool = 3 >= threshold")
	}
}

func TestIsComplex_BelowThreshold(t *testing.T) {
	outcome := SessionOutcome{
		FilesChanged: []string{"a.go"},
		ToolsUsed:    []string{"Edit"},
	}
	if isComplex(outcome) {
		t.Error("expected not complex for 1 file + 1 tool = 2 < threshold")
	}
}

func TestIsComplex_ExactlyAtThreshold(t *testing.T) {
	outcome := SessionOutcome{
		FilesChanged: []string{"a.go"},
		ToolsUsed:    []string{"Bash", "Edit"},
	}
	if !isComplex(outcome) {
		t.Error("expected complex for 1 file + 2 tools = 3 == threshold")
	}
}

func TestIsComplex_Empty(t *testing.T) {
	outcome := SessionOutcome{}
	if isComplex(outcome) {
		t.Error("expected not complex for empty outcome")
	}
}

// --- CostEntry field tests ---

func TestOnSessionEnd_CostEntry_HasSessionID(t *testing.T) {
	tracker := &mockCostTracker{}
	session := NewSession("", "", "", nil)
	lc := &SessionLifecycle{CostTracker: tracker}

	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "test",
	}

	err := lc.OnSessionEnd(context.Background(), session, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tracker.entries) != 1 {
		t.Fatal("expected 1 entry")
	}
	if !strings.HasPrefix(tracker.entries[0].SessionID, "session_") {
		t.Errorf("expected session ID prefix, got %q", tracker.entries[0].SessionID)
	}
}

func TestOnSessionEnd_CostEntry_NilSession(t *testing.T) {
	tracker := &mockCostTracker{}
	lc := &SessionLifecycle{CostTracker: tracker}

	outcome := SessionOutcome{
		Success:  true,
		TaskGoal: "test",
	}

	err := lc.OnSessionEnd(context.Background(), nil, outcome)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tracker.entries) != 1 {
		t.Fatal("expected 1 entry")
	}
	// With nil session, SessionID should be empty.
	if tracker.entries[0].SessionID != "" {
		t.Errorf("expected empty session ID for nil session, got %q", tracker.entries[0].SessionID)
	}
}
