package engine

import (
	"strings"
	"sync"
	"testing"
)

func TestGoalAddStartCompleteLifecycle(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal("Implement feature X")
	if g.ID == "" {
		t.Fatal("expected non-empty goal ID")
	}
	if g.Status != GoalPending {
		t.Fatalf("expected status pending, got %s", g.Status)
	}
	if g.Description != "Implement feature X" {
		t.Fatalf("unexpected description: %s", g.Description)
	}

	err := gt.StartGoal(g.ID)
	if err != nil {
		t.Fatalf("failed to start goal: %v", err)
	}
	if g.Status != GoalInProgress {
		t.Fatalf("expected status in_progress, got %s", g.Status)
	}
	if gt.ActiveGoal == nil || gt.ActiveGoal.ID != g.ID {
		t.Fatal("expected goal to be set as active")
	}

	err = gt.CompleteGoal(g.ID)
	if err != nil {
		t.Fatalf("failed to complete goal: %v", err)
	}
	if g.Status != GoalCompleted {
		t.Fatalf("expected status completed, got %s", g.Status)
	}
	if g.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set")
	}
	if g.Progress != 1.0 {
		t.Fatalf("expected progress 1.0, got %f", g.Progress)
	}
	if gt.ActiveGoal != nil {
		t.Fatal("expected active goal to be cleared after completion")
	}
}

func TestGoalPriorityOrdering(t *testing.T) {
	gt := NewGoalTracker()

	g1 := gt.AddGoal("Low priority task", WithPriority(5))
	g2 := gt.AddGoal("High priority task", WithPriority(1))
	g3 := gt.AddGoal("Medium priority task", WithPriority(3))

	_ = g1
	_ = g3

	next := gt.GetNextGoal()
	if next == nil {
		t.Fatal("expected a next goal")
	}
	if next.ID != g2.ID {
		t.Fatalf("expected highest priority goal (priority 1), got priority %d", next.Priority)
	}
}

func TestGoalDependencyBlocking(t *testing.T) {
	gt := NewGoalTracker()

	g1 := gt.AddGoal("First task", WithPriority(2))
	g2 := gt.AddGoal("Depends on first", WithPriority(1), WithDependencies(g1.ID))

	// g2 has higher priority but is blocked by g1
	next := gt.GetNextGoal()
	if next == nil {
		t.Fatal("expected a next goal")
	}
	if next.ID != g1.ID {
		t.Fatal("expected g1 since g2 is blocked by dependency")
	}

	// Start and complete g1
	_ = gt.StartGoal(g1.ID)
	_ = gt.CompleteGoal(g1.ID)

	// Now g2 should be available
	next = gt.GetNextGoal()
	if next == nil {
		t.Fatal("expected a next goal after dependency resolved")
	}
	if next.ID != g2.ID {
		t.Fatal("expected g2 after g1 completed")
	}

	// Trying to start a goal with unmet dependency should fail
	gt2 := NewGoalTracker()
	dep := gt2.AddGoal("dependency")
	blocked := gt2.AddGoal("blocked", WithDependencies(dep.ID))
	err := gt2.StartGoal(blocked.ID)
	if err == nil {
		t.Fatal("expected error when starting goal with unmet dependency")
	}
}

func TestTokenBudgetEnforcement(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal("Budgeted task", WithBudget(1000))

	if gt.IsBudgetExceeded(g.ID) {
		t.Fatal("budget should not be exceeded initially")
	}

	gt.RecordTokens(g.ID, 500)
	if gt.IsBudgetExceeded(g.ID) {
		t.Fatal("budget should not be exceeded at 500/1000")
	}

	gt.RecordTokens(g.ID, 500)
	if !gt.IsBudgetExceeded(g.ID) {
		t.Fatal("budget should be exceeded at 1000/1000")
	}

	gt.RecordTokens(g.ID, 100)
	if !gt.IsBudgetExceeded(g.ID) {
		t.Fatal("budget should be exceeded at 1100/1000")
	}

	// Goals with no budget should never be exceeded
	g2 := gt.AddGoal("No budget task")
	gt.RecordTokens(g2.ID, 999999)
	if gt.IsBudgetExceeded(g2.ID) {
		t.Fatal("goal with no budget should never be exceeded")
	}
}

func TestGoalDecomposition(t *testing.T) {
	gt := NewGoalTracker()

	parent := gt.AddGoal("Build feature")
	err := gt.DecomposeGoal(parent.ID, []string{
		"Design API",
		"Implement backend",
		"Write tests",
	})
	if err != nil {
		t.Fatalf("failed to decompose goal: %v", err)
	}

	if len(parent.SubGoals) != 3 {
		t.Fatalf("expected 3 sub-goals, got %d", len(parent.SubGoals))
	}

	// Complete all sub-goals and verify parent auto-completes
	for _, sub := range parent.SubGoals {
		err := gt.StartGoal(sub.ID)
		if err != nil {
			t.Fatalf("failed to start sub-goal: %v", err)
		}
		err = gt.CompleteGoal(sub.ID)
		if err != nil {
			t.Fatalf("failed to complete sub-goal: %v", err)
		}
	}

	if parent.Status != GoalCompleted {
		t.Fatalf("expected parent to be completed when all sub-goals done, got %s", parent.Status)
	}
	if parent.Progress != 1.0 {
		t.Fatalf("expected parent progress 1.0, got %f", parent.Progress)
	}
}

func TestBuildGoalContext(t *testing.T) {
	gt := NewGoalTracker()

	g1 := gt.AddGoal("Setup project", WithPriority(1), WithBudget(5000))
	g2 := gt.AddGoal("Write code", WithPriority(2), WithDependencies(g1.ID))

	_ = gt.StartGoal(g1.ID)
	gt.UpdateProgress(g1.ID, 0.45)
	gt.RecordTokens(g1.ID, 1200)

	ctx := gt.BuildGoalContext()

	if !strings.Contains(ctx, "## Current Objectives") {
		t.Fatal("expected header in context")
	}
	if !strings.Contains(ctx, "Active: Setup project") {
		t.Fatal("expected active goal in context")
	}
	if !strings.Contains(ctx, "45% complete") {
		t.Fatal("expected progress percentage in context")
	}
	if !strings.Contains(ctx, "1200/5000 tokens") {
		t.Fatal("expected token usage in context")
	}
	if !strings.Contains(ctx, "Write code") {
		t.Fatal("expected pending goal in context")
	}
	if !strings.Contains(ctx, "depends on:") {
		t.Fatal("expected dependency info in context")
	}
	if !strings.Contains(ctx, "Completed: 0/2 goals") {
		t.Fatalf("expected completion summary in context, got: %s", ctx)
	}

	// Complete g1 and check context updates
	_ = gt.CompleteGoal(g1.ID)
	ctx = gt.BuildGoalContext()
	if !strings.Contains(ctx, "Completed: 1/2 goals") {
		t.Fatalf("expected 1/2 after completing g1, got: %s", ctx)
	}
	_ = g2
}

func TestContinuationPrompt(t *testing.T) {
	gt := NewGoalTracker()

	// No goals — empty prompt
	prompt := gt.ContinuationPrompt()
	if prompt != "" {
		t.Fatal("expected empty prompt when no goals")
	}

	g := gt.AddGoal("Finish implementation")
	prompt = gt.ContinuationPrompt()
	if !strings.Contains(prompt, "remaining objectives") {
		t.Fatal("expected continuation prompt to mention remaining objectives")
	}
	if !strings.Contains(prompt, g.Description) {
		t.Fatal("expected continuation prompt to include goal description")
	}
	if !strings.Contains(prompt, "Continue working on this") {
		t.Fatal("expected continuation prompt to include instruction")
	}
}

func TestGoalsConcurrentAccess(t *testing.T) {
	gt := NewGoalTracker()

	var wg sync.WaitGroup
	goalCount := 100

	// Concurrent adds
	wg.Add(goalCount)
	for i := 0; i < goalCount; i++ {
		go func(idx int) {
			defer wg.Done()
			gt.AddGoal("concurrent goal", WithPriority(idx%5+1))
		}(i)
	}
	wg.Wait()

	if len(gt.Goals) != goalCount {
		t.Fatalf("expected %d goals, got %d", goalCount, len(gt.Goals))
	}

	// Concurrent reads while modifying
	wg.Add(goalCount * 2)
	for i := 0; i < goalCount; i++ {
		go func() {
			defer wg.Done()
			gt.GetNextGoal()
		}()
		go func() {
			defer wg.Done()
			gt.BuildGoalContext()
		}()
	}
	wg.Wait()
}

func TestFailGoalWithReason(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal("Doomed task")
	_ = gt.StartGoal(g.ID)

	err := gt.FailGoal(g.ID, "ran out of retries")
	if err != nil {
		t.Fatalf("failed to fail goal: %v", err)
	}

	if g.Status != GoalFailed {
		t.Fatalf("expected status failed, got %s", g.Status)
	}
	if gt.ActiveGoal != nil {
		t.Fatal("expected active goal to be cleared after failure")
	}

	// Verify history contains failure reason
	found := false
	for _, ev := range gt.History {
		if ev.GoalID == g.ID && ev.EventType == "failed" {
			if !strings.Contains(ev.Message, "ran out of retries") {
				t.Fatalf("expected failure reason in event, got: %s", ev.Message)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("expected a failed event in history")
	}

	// Cannot start a failed goal
	err = gt.StartGoal(g.ID)
	if err == nil {
		t.Fatal("expected error when starting a failed goal")
	}
}

func TestProgressTracking(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal("Track progress")
	if g.Progress != 0 {
		t.Fatalf("expected initial progress 0, got %f", g.Progress)
	}

	gt.UpdateProgress(g.ID, 0.5)
	if g.Progress != 0.5 {
		t.Fatalf("expected progress 0.5, got %f", g.Progress)
	}

	// Clamp to 1.0
	gt.UpdateProgress(g.ID, 1.5)
	if g.Progress != 1.0 {
		t.Fatalf("expected progress clamped to 1.0, got %f", g.Progress)
	}

	// Clamp to 0.0
	gt.UpdateProgress(g.ID, -0.5)
	if g.Progress != 0.0 {
		t.Fatalf("expected progress clamped to 0.0, got %f", g.Progress)
	}
}

func TestHistoryEventsRecording(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal("Tracked goal")
	_ = gt.StartGoal(g.ID)
	gt.UpdateProgress(g.ID, 0.5)
	_ = gt.CompleteGoal(g.ID)

	expectedTypes := []string{"created", "started", "progress", "completed"}
	if len(gt.History) < len(expectedTypes) {
		t.Fatalf("expected at least %d events, got %d", len(expectedTypes), len(gt.History))
	}

	for i, expected := range expectedTypes {
		if gt.History[i].EventType != expected {
			t.Fatalf("event %d: expected type %s, got %s", i, expected, gt.History[i].EventType)
		}
		if gt.History[i].GoalID != g.ID {
			t.Fatalf("event %d: expected goal ID %s, got %s", i, g.ID, gt.History[i].GoalID)
		}
		if gt.History[i].Timestamp.IsZero() {
			t.Fatalf("event %d: expected non-zero timestamp", i)
		}
	}
}

func TestGetNextGoalPrefersInProgress(t *testing.T) {
	gt := NewGoalTracker()

	_ = gt.AddGoal("High priority pending", WithPriority(1))
	g2 := gt.AddGoal("Lower priority in progress", WithPriority(3))

	_ = gt.StartGoal(g2.ID)

	next := gt.GetNextGoal()
	if next == nil {
		t.Fatal("expected a next goal")
	}
	if next.ID != g2.ID {
		t.Fatal("expected in_progress goal to be preferred over pending")
	}
}

func TestGoalOptions(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal(
		"Options test",
		WithPriority(1),
		WithBudget(2000),
		WithDependencies("dep-1", "dep-2"),
		WithTags("feature", "v2"),
	)

	if g.Priority != 1 {
		t.Fatalf("expected priority 1, got %d", g.Priority)
	}
	if g.TokenBudget != 2000 {
		t.Fatalf("expected budget 2000, got %d", g.TokenBudget)
	}
	if len(g.Dependencies) != 2 || g.Dependencies[0] != "dep-1" || g.Dependencies[1] != "dep-2" {
		t.Fatalf("unexpected dependencies: %v", g.Dependencies)
	}
	if len(g.Tags) != 2 || g.Tags[0] != "feature" || g.Tags[1] != "v2" {
		t.Fatalf("unexpected tags: %v", g.Tags)
	}
}

func TestDecomposeGoalEmptyDescriptions(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal("Parent")
	err := gt.DecomposeGoal(g.ID, []string{})
	if err == nil {
		t.Fatal("expected error for empty sub-descriptions")
	}
}

func TestDecomposeGoalNotFound(t *testing.T) {
	gt := NewGoalTracker()

	err := gt.DecomposeGoal("nonexistent", []string{"sub"})
	if err == nil {
		t.Fatal("expected error for nonexistent goal")
	}
}

func TestCompleteGoalNotFound(t *testing.T) {
	gt := NewGoalTracker()

	err := gt.CompleteGoal("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent goal")
	}
}

func TestStartGoalNotFound(t *testing.T) {
	gt := NewGoalTracker()

	err := gt.StartGoal("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent goal")
	}
}

func TestFailGoalNotFound(t *testing.T) {
	gt := NewGoalTracker()

	err := gt.FailGoal("nonexistent", "reason")
	if err == nil {
		t.Fatal("expected error for nonexistent goal")
	}
}

func TestCannotStartCompletedGoal(t *testing.T) {
	gt := NewGoalTracker()

	g := gt.AddGoal("Task")
	_ = gt.StartGoal(g.ID)
	_ = gt.CompleteGoal(g.ID)

	err := gt.StartGoal(g.ID)
	if err == nil {
		t.Fatal("expected error when starting a completed goal")
	}
}

func TestBudgetExceededGoalSkippedByGetNext(t *testing.T) {
	gt := NewGoalTracker()

	g1 := gt.AddGoal("Over budget", WithPriority(1), WithBudget(100))
	g2 := gt.AddGoal("Under budget", WithPriority(2), WithBudget(1000))

	gt.RecordTokens(g1.ID, 200) // exceed budget

	next := gt.GetNextGoal()
	if next == nil {
		t.Fatal("expected a next goal")
	}
	if next.ID != g2.ID {
		t.Fatal("expected budget-exceeded goal to be skipped")
	}
}
