package streaming

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewThinkingProtocol(t *testing.T) {
	tp := NewThinkingProtocol()
	if tp == nil {
		t.Fatal("NewThinkingProtocol returned nil")
	}
	if !tp.Enabled {
		t.Error("expected Enabled to be true")
	}
	if !tp.Visible {
		t.Error("expected Visible to be true")
	}
	if len(tp.Steps) != 0 {
		t.Error("expected empty Steps slice")
	}
	if tp.CurrentPhase != "" {
		t.Error("expected empty CurrentPhase")
	}
}

func TestPhaseTransitions(t *testing.T) {
	tp := NewThinkingProtocol()

	phases := []ThinkingPhase{PhaseUnderstand, PhasePlan, PhaseExecute, PhaseVerify, PhaseReflect}
	for _, phase := range phases {
		tp.StartPhase(phase)
		if tp.CurrentPhase != phase {
			t.Errorf("expected CurrentPhase=%s, got %s", phase, tp.CurrentPhase)
		}
	}
}

func TestAddThoughtRecording(t *testing.T) {
	tp := NewThinkingProtocol()

	tp.StartPhase(PhaseUnderstand)
	tp.AddThought("Need to parse user input", 0.9)

	if len(tp.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(tp.Steps))
	}

	step := tp.Steps[0]
	if step.Phase != PhaseUnderstand {
		t.Errorf("expected phase %s, got %s", PhaseUnderstand, step.Phase)
	}
	if step.Content != "Need to parse user input" {
		t.Errorf("unexpected content: %s", step.Content)
	}
	if step.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", step.Confidence)
	}
	if step.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestAddThoughtMultiplePhases(t *testing.T) {
	tp := NewThinkingProtocol()

	tp.StartPhase(PhaseUnderstand)
	tp.AddThought("Understanding the task", 0.95)

	tp.StartPhase(PhasePlan)
	tp.AddThought("Will modify two files", 0.85)
	tp.AddThought("Start with tests", 0.80)

	if len(tp.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(tp.Steps))
	}

	if tp.Steps[0].Phase != PhaseUnderstand {
		t.Error("first step should be understand phase")
	}
	if tp.Steps[1].Phase != PhasePlan {
		t.Error("second step should be plan phase")
	}
	if tp.Steps[2].Phase != PhasePlan {
		t.Error("third step should be plan phase")
	}
}

func TestBuildThinkingPrompt(t *testing.T) {
	tp := NewThinkingProtocol()
	task := "Add authentication to the API"
	prompt := tp.BuildThinkingPrompt(task)

	expectedParts := []string{
		"Before implementing, think through this systematically:",
		"1. UNDERSTAND:",
		"2. PLAN:",
		"3. RISKS:",
		"4. EXECUTE:",
		"5. VERIFY:",
		"Task: Add authentication to the API",
		"Begin with your understanding, then plan, then execute.",
	}

	for _, part := range expectedParts {
		if !strings.Contains(prompt, part) {
			t.Errorf("prompt missing expected part: %q", part)
		}
	}
}

func TestParseThinkingExtractsPhases(t *testing.T) {
	response := `Understanding: The user wants to add JWT auth to API routes.
Plan: Create middleware, add to router, update tests.
Risks: Breaking existing endpoints.
Execute: Starting with middleware creation.`

	tp := NewThinkingProtocol()
	steps := tp.ParseThinking(response)

	if len(steps) < 3 {
		t.Fatalf("expected at least 3 steps, got %d", len(steps))
	}

	// Check that phases are correctly identified
	foundPhases := make(map[ThinkingPhase]bool)
	for _, step := range steps {
		foundPhases[step.Phase] = true
	}

	if !foundPhases[PhaseUnderstand] {
		t.Error("missing understand phase")
	}
	if !foundPhases[PhasePlan] {
		t.Error("missing plan phase")
	}
	if !foundPhases[PhaseExecute] {
		t.Error("missing execute phase")
	}
}

func TestParseThinkingWithThinkingTags(t *testing.T) {
	response := `Some preamble.
<thinking>
Understanding: The task requires refactoring.
Plan: Split into smaller functions.
</thinking>
Some output.`

	tp := NewThinkingProtocol()
	steps := tp.ParseThinking(response)

	if len(steps) < 2 {
		t.Fatalf("expected at least 2 steps, got %d", len(steps))
	}

	foundUnderstand := false
	foundPlan := false
	for _, step := range steps {
		if step.Phase == PhaseUnderstand {
			foundUnderstand = true
		}
		if step.Phase == PhasePlan {
			foundPlan = true
		}
	}
	if !foundUnderstand {
		t.Error("missing understand phase from thinking tags")
	}
	if !foundPlan {
		t.Error("missing plan phase from thinking tags")
	}
}

func TestShouldThinkFirst_SimpleTask(t *testing.T) {
	tp := NewThinkingProtocol()

	// Short simple task
	if tp.ShouldThinkFirst("fix typo") {
		t.Error("simple task should not need thinking")
	}

	// Very short
	if tp.ShouldThinkFirst("hello") {
		t.Error("greeting should not need thinking")
	}
}

func TestShouldThinkFirst_ComplexKeywords(t *testing.T) {
	tp := NewThinkingProtocol()

	complexTasks := []string{
		"Refactor the authentication module to use JWT",
		"Redesign the database schema for better performance",
		"Migrate from MySQL to PostgreSQL",
		"Implement a new caching layer with Redis",
	}

	for _, task := range complexTasks {
		if !tp.ShouldThinkFirst(task) {
			t.Errorf("expected thinking for complex task: %s", task)
		}
	}
}

func TestShouldThinkFirst_LongPrompt(t *testing.T) {
	tp := NewThinkingProtocol()

	// Generate a prompt with >100 words
	words := make([]string, 105)
	for i := range words {
		words[i] = "word"
	}
	longTask := strings.Join(words, " ")

	if !tp.ShouldThinkFirst(longTask) {
		t.Error("long prompt should trigger thinking")
	}
}

func TestShouldThinkFirst_MultiFile(t *testing.T) {
	tp := NewThinkingProtocol()

	task := "Update handler.go and service.go to add the new endpoint"
	if !tp.ShouldThinkFirst(task) {
		t.Error("multi-file task should trigger thinking")
	}
}

func TestFormatThinking(t *testing.T) {
	tp := NewThinkingProtocol()

	steps := []ThinkingStep{
		{
			Phase:      PhaseUnderstand,
			Content:    "Need to add JWT authentication to the API endpoints.",
			Confidence: 0.95,
		},
		{
			Phase:        PhasePlan,
			Content:      "1. Create auth middleware\n2. Add to router\n3. Update tests",
			Confidence:   0.85,
			Alternatives: []string{"OAuth2 (rejected: overkill for this use case)"},
		},
	}

	output := tp.FormatThinking(steps)

	if !strings.Contains(output, "Thinking Process:") {
		t.Error("missing thinking process header")
	}
	if !strings.Contains(output, "Understanding (confidence: 0.95):") {
		t.Error("missing understanding section with confidence")
	}
	if !strings.Contains(output, "Plan (confidence: 0.85):") {
		t.Error("missing plan section with confidence")
	}
	if !strings.Contains(output, "JWT authentication") {
		t.Error("missing understanding content")
	}
	if !strings.Contains(output, "Create auth middleware") {
		t.Error("missing plan content")
	}
	if !strings.Contains(output, "Alternative considered: OAuth2") {
		t.Error("missing alternatives")
	}
}

func TestFormatThinkingEmpty(t *testing.T) {
	tp := NewThinkingProtocol()
	output := tp.FormatThinking(nil)
	if output != "" {
		t.Error("expected empty output for nil steps")
	}
}

func TestSummarizeThinking(t *testing.T) {
	tp := NewThinkingProtocol()

	// No steps
	summary := tp.SummarizeThinking()
	if summary != "No thinking recorded." {
		t.Errorf("unexpected empty summary: %s", summary)
	}

	// Add some steps
	tp.StartPhase(PhaseUnderstand)
	tp.AddThought("Understanding the task", 0.9)
	tp.StartPhase(PhasePlan)
	tp.AddThought("Planning approach", 0.8)

	summary = tp.SummarizeThinking()
	if !strings.Contains(summary, "2 steps") {
		t.Errorf("summary should mention 2 steps: %s", summary)
	}
	if !strings.Contains(summary, "understand") {
		t.Errorf("summary should mention understand phase: %s", summary)
	}
	if !strings.Contains(summary, "plan") {
		t.Errorf("summary should mention plan phase: %s", summary)
	}
	// Average confidence should be 0.85
	if !strings.Contains(summary, "0.85") {
		t.Errorf("summary should contain avg confidence 0.85: %s", summary)
	}
}

func TestSummarizeThinkingConciseness(t *testing.T) {
	tp := NewThinkingProtocol()
	tp.StartPhase(PhaseUnderstand)
	tp.AddThought("A very long thought about understanding the complex nature of the task at hand", 0.9)
	tp.StartPhase(PhasePlan)
	tp.AddThought("A detailed plan with many steps that could be verbose", 0.8)
	tp.StartPhase(PhaseExecute)
	tp.AddThought("Execution details", 0.7)

	summary := tp.SummarizeThinking()
	// Should be one line, relatively concise
	lines := strings.Split(summary, "\n")
	if len(lines) != 1 {
		t.Errorf("summary should be a single line, got %d lines", len(lines))
	}
	if len(summary) > 200 {
		t.Errorf("summary too verbose (%d chars): %s", len(summary), summary)
	}
}

func TestConfidenceTracking(t *testing.T) {
	tp := NewThinkingProtocol()

	tp.StartPhase(PhaseUnderstand)
	tp.AddThought("High confidence understanding", 0.99)
	tp.AddThought("Lower confidence detail", 0.60)

	if tp.Steps[0].Confidence != 0.99 {
		t.Errorf("expected 0.99, got %f", tp.Steps[0].Confidence)
	}
	if tp.Steps[1].Confidence != 0.60 {
		t.Errorf("expected 0.60, got %f", tp.Steps[1].Confidence)
	}
}

func TestAlternativesRecording(t *testing.T) {
	tp := NewThinkingProtocol()

	tp.StartPhase(PhasePlan)
	tp.AddThought("Use REST API", 0.85)
	tp.AddAlternative("Use REST API", "GraphQL (too complex for this case)")
	tp.AddAlternative("Use REST API", "gRPC (overkill for internal service)")

	if len(tp.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(tp.Steps))
	}
	if len(tp.Steps[0].Alternatives) != 2 {
		t.Fatalf("expected 2 alternatives, got %d", len(tp.Steps[0].Alternatives))
	}
	if tp.Steps[0].Alternatives[0] != "GraphQL (too complex for this case)" {
		t.Errorf("unexpected alternative: %s", tp.Steps[0].Alternatives[0])
	}
	if tp.Steps[0].Alternatives[1] != "gRPC (overkill for internal service)" {
		t.Errorf("unexpected alternative: %s", tp.Steps[0].Alternatives[1])
	}
}

func TestAlternativeNotFound(t *testing.T) {
	tp := NewThinkingProtocol()

	tp.StartPhase(PhasePlan)
	tp.AddThought("Some thought", 0.8)
	// Adding alternative for non-existent thought should not panic
	tp.AddAlternative("Non-existent thought", "Some alternative")

	// The alternative should not be added to the existing thought
	if len(tp.Steps[0].Alternatives) != 0 {
		t.Error("alternative should not be added to wrong thought")
	}
}

func TestResetForNewTask(t *testing.T) {
	tp := NewThinkingProtocol()

	tp.StartPhase(PhaseUnderstand)
	tp.AddThought("Some understanding", 0.9)
	tp.StartPhase(PhasePlan)
	tp.AddThought("Some plan", 0.85)

	if len(tp.Steps) != 2 {
		t.Fatal("setup failed")
	}

	tp.ResetForNewTask()

	if len(tp.Steps) != 0 {
		t.Errorf("expected empty Steps after reset, got %d", len(tp.Steps))
	}
	if tp.CurrentPhase != "" {
		t.Errorf("expected empty CurrentPhase after reset, got %s", tp.CurrentPhase)
	}
}

func TestGetPhaseHistory(t *testing.T) {
	tp := NewThinkingProtocol()

	tp.StartPhase(PhaseUnderstand)
	tp.AddThought("Understanding 1", 0.9)
	tp.AddThought("Understanding 2", 0.85)
	tp.StartPhase(PhasePlan)
	tp.AddThought("Plan 1", 0.8)
	tp.StartPhase(PhaseVerify)
	tp.AddThought("Verify 1", 0.95)

	history := tp.GetPhaseHistory()

	if len(history[PhaseUnderstand]) != 2 {
		t.Errorf("expected 2 understand steps, got %d", len(history[PhaseUnderstand]))
	}
	if len(history[PhasePlan]) != 1 {
		t.Errorf("expected 1 plan step, got %d", len(history[PhasePlan]))
	}
	if len(history[PhaseVerify]) != 1 {
		t.Errorf("expected 1 verify step, got %d", len(history[PhaseVerify]))
	}
	if len(history[PhaseExecute]) != 0 {
		t.Errorf("expected 0 execute steps, got %d", len(history[PhaseExecute]))
	}
}

func TestThinkingConcurrentAccess(t *testing.T) {
	tp := NewThinkingProtocol()
	tp.StartPhase(PhasePlan)

	var wg sync.WaitGroup
	iterations := 100

	// Concurrent writers
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func(n int) {
			defer wg.Done()
			tp.AddThought("concurrent thought", 0.5)
		}(i)
	}

	// Concurrent readers
	wg.Add(iterations)
	for i := 0; i < iterations; i++ {
		go func() {
			defer wg.Done()
			_ = tp.SummarizeThinking()
			_ = tp.GetPhaseHistory()
		}()
	}

	// Concurrent phase transitions
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer wg.Done()
			phases := []ThinkingPhase{PhaseUnderstand, PhasePlan, PhaseExecute, PhaseVerify, PhaseReflect}
			tp.StartPhase(phases[n%len(phases)])
		}(i)
	}

	// Concurrent resets
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond)
			tp.ResetForNewTask()
		}()
	}

	wg.Wait()
	// If we get here without a race condition panic, the test passes
}

func TestParseThinkingUppercaseMarkers(t *testing.T) {
	response := `UNDERSTAND: The task is to build a CLI tool.
PLAN: Use cobra for command structure.
EXECUTE: Create main.go and cmd package.
VERIFY: Run go build and test.`

	tp := NewThinkingProtocol()
	steps := tp.ParseThinking(response)

	if len(steps) < 4 {
		t.Fatalf("expected at least 4 steps, got %d", len(steps))
	}

	phases := make(map[ThinkingPhase]bool)
	for _, step := range steps {
		phases[step.Phase] = true
	}

	if !phases[PhaseUnderstand] {
		t.Error("missing understand phase")
	}
	if !phases[PhasePlan] {
		t.Error("missing plan phase")
	}
	if !phases[PhaseExecute] {
		t.Error("missing execute phase")
	}
	if !phases[PhaseVerify] {
		t.Error("missing verify phase")
	}
}

func TestThinkingStepDuration(t *testing.T) {
	tp := NewThinkingProtocol()
	tp.StartPhase(PhaseUnderstand)

	time.Sleep(5 * time.Millisecond)
	tp.AddThought("After a short delay", 0.9)

	if tp.Steps[0].Duration < 5*time.Millisecond {
		t.Errorf("expected duration >= 5ms, got %v", tp.Steps[0].Duration)
	}
}
