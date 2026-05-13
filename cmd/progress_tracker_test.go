package cmd

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewProgressTracker(t *testing.T) {
	pt := NewProgressTracker("Test Task")
	if pt.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %q", pt.Title)
	}
	if pt.CurrentStep != -1 {
		t.Errorf("expected CurrentStep -1, got %d", pt.CurrentStep)
	}
	if pt.StartTime.IsZero() {
		t.Error("expected non-zero StartTime")
	}
	if len(pt.Steps) != 0 {
		t.Errorf("expected 0 steps, got %d", len(pt.Steps))
	}
}

func TestAddStep(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.AddStep("Step 2")
	pt.AddStep("Step 3")

	if len(pt.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(pt.Steps))
	}
	if pt.Steps[0].Name != "Step 1" {
		t.Errorf("expected step name 'Step 1', got %q", pt.Steps[0].Name)
	}
	if pt.Steps[0].Status != "pending" {
		t.Errorf("expected status 'pending', got %q", pt.Steps[0].Status)
	}
}

func TestStartStep(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.StartStep(0)

	if pt.Steps[0].Status != "active" {
		t.Errorf("expected status 'active', got %q", pt.Steps[0].Status)
	}
	if pt.Steps[0].StartTime == nil {
		t.Error("expected non-nil StartTime")
	}
	if pt.CurrentStep != 0 {
		t.Errorf("expected CurrentStep 0, got %d", pt.CurrentStep)
	}
}

func TestStartStepOutOfBounds(t *testing.T) {
	pt := NewProgressTracker("Task")
	// Should not panic
	pt.StartStep(-1)
	pt.StartStep(0)
	pt.StartStep(100)
}

func TestCompleteStep(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.StartStep(0)

	time.Sleep(10 * time.Millisecond)
	pt.CompleteStep(0)

	if pt.Steps[0].Status != "done" {
		t.Errorf("expected status 'done', got %q", pt.Steps[0].Status)
	}
	if pt.Steps[0].EndTime == nil {
		t.Error("expected non-nil EndTime")
	}
	if pt.Steps[0].Duration <= 0 {
		t.Errorf("expected positive duration, got %v", pt.Steps[0].Duration)
	}
	if pt.Steps[0].Progress != 1.0 {
		t.Errorf("expected progress 1.0, got %f", pt.Steps[0].Progress)
	}
}

func TestFailStep(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.StartStep(0)

	time.Sleep(5 * time.Millisecond)
	pt.FailStep(0, "network timeout")

	if pt.Steps[0].Status != "failed" {
		t.Errorf("expected status 'failed', got %q", pt.Steps[0].Status)
	}
	if pt.Steps[0].failReason != "network timeout" {
		t.Errorf("expected fail reason 'network timeout', got %q", pt.Steps[0].failReason)
	}
	if pt.Steps[0].EndTime == nil {
		t.Error("expected non-nil EndTime")
	}
	if pt.Steps[0].Duration <= 0 {
		t.Errorf("expected positive duration, got %v", pt.Steps[0].Duration)
	}
}

func TestSkipStep(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.SkipStep(0)

	if pt.Steps[0].Status != "skipped" {
		t.Errorf("expected status 'skipped', got %q", pt.Steps[0].Status)
	}
}

func TestSetProgress(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")

	pt.SetProgress(0, 0.5)
	if pt.Steps[0].Progress != 0.5 {
		t.Errorf("expected progress 0.5, got %f", pt.Steps[0].Progress)
	}

	// Clamp to 0
	pt.SetProgress(0, -0.1)
	if pt.Steps[0].Progress != 0 {
		t.Errorf("expected progress 0, got %f", pt.Steps[0].Progress)
	}

	// Clamp to 1.0
	pt.SetProgress(0, 1.5)
	if pt.Steps[0].Progress != 1.0 {
		t.Errorf("expected progress 1.0, got %f", pt.Steps[0].Progress)
	}
}

func TestSetProgressOutOfBounds(t *testing.T) {
	pt := NewProgressTracker("Task")
	// Should not panic
	pt.SetProgress(-1, 0.5)
	pt.SetProgress(100, 0.5)
}

func TestAddSubstep(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.AddSubstep(0, "Created file.go")
	pt.AddSubstep(0, "Added validation")

	if len(pt.Steps[0].Substeps) != 2 {
		t.Fatalf("expected 2 substeps, got %d", len(pt.Steps[0].Substeps))
	}
	if pt.Steps[0].Substeps[0] != "Created file.go" {
		t.Errorf("expected substep 'Created file.go', got %q", pt.Steps[0].Substeps[0])
	}
}

func TestAddSubstepOutOfBounds(t *testing.T) {
	pt := NewProgressTracker("Task")
	// Should not panic
	pt.AddSubstep(-1, "foo")
	pt.AddSubstep(100, "bar")
}

func TestRender(t *testing.T) {
	pt := NewProgressTracker("Implementing JWT Authentication")
	pt.AddStep("Read existing auth code")
	pt.AddStep("Plan approach")
	pt.AddStep("Implement JWT middleware")
	pt.AddStep("Write tests")
	pt.AddStep("Verify and commit")

	// Complete first two steps
	pt.StartStep(0)
	pt.CompleteStep(0)
	pt.StartStep(1)
	pt.CompleteStep(1)

	// Start third step with substeps
	pt.StartStep(2)
	pt.AddSubstep(2, "Created token.go")
	pt.AddSubstep(2, "Added validation logic")
	pt.AddSubstep(2, "Working on expiry handling")
	pt.SetProgress(2, 0.65)

	output := pt.Render()

	// Check title is present
	if !strings.Contains(output, "Implementing JWT Authentication") {
		t.Error("render missing title")
	}

	// Check completed steps have checkmark
	if !strings.Contains(output, "✓ Read existing auth code") {
		t.Error("render missing completed step icon")
	}

	// Check active step has bullet
	if !strings.Contains(output, "● Implement JWT middleware") {
		t.Error("render missing active step icon")
	}

	// Check pending steps have circle
	if !strings.Contains(output, "○ Write tests") {
		t.Error("render missing pending step icon")
	}

	// Check substeps
	if !strings.Contains(output, "├─ Created token.go") {
		t.Error("render missing substep with tree connector")
	}
	if !strings.Contains(output, "└─ Working on expiry handling... (65%)") {
		t.Error("render missing last substep with progress")
	}

	// Check progress bar
	if !strings.Contains(output, "Progress: [") {
		t.Error("render missing progress bar")
	}

	// Check separator
	if !strings.Contains(output, "────") {
		t.Error("render missing separator")
	}
}

func TestRenderWithFailedStep(t *testing.T) {
	pt := NewProgressTracker("Deploy")
	pt.AddStep("Build")
	pt.AddStep("Test")

	pt.StartStep(0)
	pt.CompleteStep(0)
	pt.StartStep(1)
	pt.FailStep(1, "assertion error in test_foo")

	output := pt.Render()

	if !strings.Contains(output, "✗ Test") {
		t.Error("render missing failed step icon")
	}
	if !strings.Contains(output, "assertion error in test_foo") {
		t.Error("render missing fail reason")
	}
}

func TestRenderCompact(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step A")
	pt.AddStep("Step B")
	pt.AddStep("Step C")

	pt.StartStep(0)
	pt.CompleteStep(0)
	pt.StartStep(1)
	pt.SetProgress(1, 0.55)

	output := pt.RenderCompact()

	// Should show done count / total
	if !strings.Contains(output, "[1/3]") {
		t.Errorf("compact render missing step count, got: %q", output)
	}
	// Should show current step name
	if !strings.Contains(output, "Step B...") {
		t.Errorf("compact render missing step name, got: %q", output)
	}
	// Should show percentage
	if !strings.Contains(output, "55%") {
		t.Errorf("compact render missing progress pct, got: %q", output)
	}
}

func TestRenderDone(t *testing.T) {
	pt := NewProgressTracker("JWT Authentication")
	pt.AddStep("Step 1")
	pt.AddStep("Step 2")
	pt.AddStep("Step 3")

	pt.StartStep(0)
	pt.CompleteStep(0)
	pt.StartStep(1)
	pt.CompleteStep(1)
	pt.StartStep(2)
	pt.CompleteStep(2)

	output := pt.RenderDone()

	if !strings.Contains(output, "✓ JWT Authentication complete") {
		t.Errorf("done render missing completion message, got: %q", output)
	}
	if !strings.Contains(output, "3 steps, 0 failures") {
		t.Errorf("done render missing stats, got: %q", output)
	}
}

func TestRenderDoneWithFailures(t *testing.T) {
	pt := NewProgressTracker("Deploy")
	pt.AddStep("Build")
	pt.AddStep("Test")
	pt.AddStep("Push")

	pt.StartStep(0)
	pt.CompleteStep(0)
	pt.StartStep(1)
	pt.FailStep(1, "timeout")
	pt.SkipStep(2)

	output := pt.RenderDone()

	if !strings.Contains(output, "3 steps, 1 failures") {
		t.Errorf("done render missing failure count, got: %q", output)
	}
}

func TestIsComplete(t *testing.T) {
	pt := NewProgressTracker("Task")

	// No steps => not complete
	if pt.IsComplete() {
		t.Error("empty tracker should not be complete")
	}

	pt.AddStep("Step 1")
	pt.AddStep("Step 2")

	// Pending steps => not complete
	if pt.IsComplete() {
		t.Error("tracker with pending steps should not be complete")
	}

	pt.StartStep(0)
	// Active step => not complete
	if pt.IsComplete() {
		t.Error("tracker with active step should not be complete")
	}

	pt.CompleteStep(0)
	pt.SkipStep(1)

	// All done/skipped => complete
	if !pt.IsComplete() {
		t.Error("tracker with all done/skipped steps should be complete")
	}
}

func TestIsCompleteWithFailure(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.StartStep(0)
	pt.FailStep(0, "error")

	if !pt.IsComplete() {
		t.Error("tracker with all failed steps should be complete")
	}
}

func TestGetElapsed(t *testing.T) {
	pt := NewProgressTracker("Task")
	time.Sleep(10 * time.Millisecond)
	elapsed := pt.GetElapsed()
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected elapsed >= 10ms, got %v", elapsed)
	}
}

func TestEstimateRemaining(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.AddStep("Step 2")
	pt.AddStep("Step 3")

	// No completed steps, no active progress => 0
	remaining := pt.EstimateRemaining()
	if remaining != 0 {
		t.Errorf("expected 0 remaining with no data, got %v", remaining)
	}

	// Complete one step with a known duration
	now := time.Now()
	past := now.Add(-100 * time.Millisecond)
	pt.mu.Lock()
	pt.Steps[0].Status = "done"
	pt.Steps[0].StartTime = &past
	pt.Steps[0].EndTime = &now
	pt.Steps[0].Duration = 100 * time.Millisecond
	pt.mu.Unlock()

	// 2 pending steps remaining, avg duration is 100ms => ~200ms estimate
	remaining = pt.EstimateRemaining()
	if remaining < 150*time.Millisecond || remaining > 250*time.Millisecond {
		t.Errorf("expected remaining ~200ms, got %v", remaining)
	}
}

func TestEstimateRemainingWithActiveProgress(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.AddStep("Step 2")

	// Set up step 0 as active at 50% progress, started 100ms ago
	now := time.Now()
	start := now.Add(-100 * time.Millisecond)
	pt.mu.Lock()
	pt.Steps[0].Status = "active"
	pt.Steps[0].StartTime = &start
	pt.Steps[0].Progress = 0.5
	pt.mu.Unlock()

	remaining := pt.EstimateRemaining()
	// At 50% progress, 100ms elapsed => ~100ms remaining for active step
	// Plus no completed steps to estimate pending from => just active remaining
	if remaining < 50*time.Millisecond {
		t.Errorf("expected some remaining time from active step, got %v", remaining)
	}
}

func TestProgressTrackerConcurrentAccess(t *testing.T) {
	pt := NewProgressTracker("Concurrent Task")
	pt.AddStep("Step 1")
	pt.AddStep("Step 2")
	pt.AddStep("Step 3")

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		pt.StartStep(0)
		pt.SetProgress(0, 0.5)
		pt.AddSubstep(0, "sub1")
		pt.CompleteStep(0)
	}()

	go func() {
		defer wg.Done()
		pt.StartStep(1)
		pt.SetProgress(1, 0.3)
		pt.FailStep(1, "error")
	}()

	go func() {
		defer wg.Done()
		pt.SkipStep(2)
	}()

	go func() {
		defer wg.Done()
		_ = pt.Render()
		_ = pt.RenderCompact()
		_ = pt.RenderDone()
		_ = pt.IsComplete()
		_ = pt.GetElapsed()
		_ = pt.EstimateRemaining()
	}()

	wg.Wait()

	// Should not panic - verify tracker is still usable
	if !pt.IsComplete() {
		t.Error("all steps should be terminal after concurrent ops")
	}
}

func TestFormatDurationShort(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "500ms"},
		{1 * time.Second, "1.0s"},
		{1500 * time.Millisecond, "1.5s"},
		{30 * time.Second, "30.0s"},
		{90 * time.Second, "1m 30s"},
		{5 * time.Minute, "5m"},
		{3*time.Minute + 45*time.Second, "3m 45s"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
	}

	for _, tt := range tests {
		got := formatDurationShort(tt.d)
		if got != tt.want {
			t.Errorf("formatDurationShort(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestOverallProgress(t *testing.T) {
	pt := NewProgressTracker("Task")
	pt.AddStep("Step 1")
	pt.AddStep("Step 2")
	pt.AddStep("Step 3")
	pt.AddStep("Step 4")

	// 1 done, 1 active at 50%, 2 pending => (1.0 + 0.5 + 0 + 0) / 4 = 0.375
	pt.mu.Lock()
	pt.Steps[0].Status = "done"
	pt.Steps[1].Status = "active"
	pt.Steps[1].Progress = 0.5
	pt.mu.Unlock()

	pt.mu.Lock()
	progress := pt.overallProgress()
	pt.mu.Unlock()

	expected := 0.375
	if progress < expected-0.01 || progress > expected+0.01 {
		t.Errorf("expected overall progress ~%.3f, got %.3f", expected, progress)
	}
}

func TestRenderNoSteps(t *testing.T) {
	pt := NewProgressTracker("Empty Task")
	output := pt.Render()

	if !strings.Contains(output, "Empty Task") {
		t.Error("render missing title for empty tracker")
	}
	if !strings.Contains(output, "Progress: [") {
		t.Error("render missing progress bar for empty tracker")
	}
}

func TestRenderCompactNoSteps(t *testing.T) {
	pt := NewProgressTracker("Empty")
	output := pt.RenderCompact()
	if !strings.Contains(output, "[0/0]") {
		t.Errorf("compact render unexpected output for empty tracker: %q", output)
	}
}
