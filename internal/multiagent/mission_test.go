package mission

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	m := New("Build auth system", Config{MaxWorkers: 2, BaseBranch: "main"})
	if m.ID == "" {
		t.Error("expected non-empty mission ID")
	}
	if m.Status != StatusPlanning {
		t.Errorf("expected planning status, got %s", m.Status)
	}
	if m.Config.MaxWorkers != 2 {
		t.Errorf("expected 2 workers, got %d", m.Config.MaxWorkers)
	}
}

func TestNew_Defaults(t *testing.T) {
	m := New("test", Config{})
	if m.Config.MaxWorkers != 4 {
		t.Errorf("expected default 4 workers, got %d", m.Config.MaxWorkers)
	}
	if m.Config.BaseBranch != "main" {
		t.Errorf("expected default branch 'main', got %s", m.Config.BaseBranch)
	}
}

func TestMission_Plan(t *testing.T) {
	m := New("test", Config{})

	planFn := func(_ context.Context, prompt string) ([]Feature, error) {
		return []Feature{
			{Description: "Feature A"},
			{Description: "Feature B"},
		}, nil
	}

	if err := m.Plan(context.Background(), planFn); err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(m.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(m.Features))
	}
	if m.Features[0].ID != "feat-1" {
		t.Errorf("expected feat-1, got %s", m.Features[0].ID)
	}
	if m.Features[1].Branch == "" {
		t.Error("expected non-empty branch name")
	}
	if m.Features[0].Status != FeaturePending {
		t.Errorf("expected pending, got %s", m.Features[0].Status)
	}
}

func TestMission_Run(t *testing.T) {
	m := New("test", Config{MaxWorkers: 2})
	m.Features = []Feature{
		{ID: "f1", Description: "A", Status: FeaturePending},
		{ID: "f2", Description: "B", Status: FeaturePending},
		{ID: "f3", Description: "C", Status: FeaturePending},
	}

	var count atomic.Int32
	workerFn := func(_ context.Context, feat *Feature, _ string, _ Config) (*Handoff, error) {
		count.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &Handoff{Summary: "done: " + feat.ID, TestsPassed: true}, nil
	}

	if err := m.Run(context.Background(), workerFn); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if count.Load() != 3 {
		t.Errorf("expected 3 workers to run, got %d", count.Load())
	}
	if m.Status != StatusCompleted {
		t.Errorf("expected completed, got %s", m.Status)
	}
	for _, f := range m.Features {
		if f.Status != FeatureCompleted {
			t.Errorf("feature %s: expected completed, got %s", f.ID, f.Status)
		}
		if f.Handoff == nil {
			t.Errorf("feature %s: missing handoff", f.ID)
		}
	}
}

func TestMission_Run_PartialFailure(t *testing.T) {
	m := New("test", Config{MaxWorkers: 4})
	m.Features = []Feature{
		{ID: "f1", Description: "OK", Status: FeaturePending},
		{ID: "f2", Description: "FAIL", Status: FeaturePending},
	}

	workerFn := func(_ context.Context, feat *Feature, _ string, _ Config) (*Handoff, error) {
		if feat.ID == "f2" {
			return nil, context.DeadlineExceeded
		}
		return &Handoff{Summary: "ok"}, nil
	}

	if err := m.Run(context.Background(), workerFn); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if m.Status != "partial" {
		t.Errorf("expected partial, got %s", m.Status)
	}
	if m.Features[0].Status != FeatureCompleted {
		t.Errorf("f1 should complete even when f2 fails")
	}
	if m.Features[1].Status != FeatureFailed {
		t.Errorf("f2 should be failed")
	}
}

// TestMission_Run_RetryUsesAttemptSuffixedBranch verifies the H9 fix: each
// retry attempt gets a unique branch name so `git worktree add -b` can never
// collide with the previous attempt's branch.
func TestMission_Run_RetryUsesAttemptSuffixedBranch(t *testing.T) {
	m := New("test", Config{MaxWorkers: 2, MaxRetriesPerFeat: 1})
	m.Features = []Feature{
		{ID: "f1", Description: "A", Status: FeaturePending},
	}

	seen := make([]string, 0, 2)
	var mu sync.Mutex
	attempts := 0
	workerFn := func(_ context.Context, feat *Feature, _ string, _ Config) (*Handoff, error) {
		mu.Lock()
		seen = append(seen, feat.Branch)
		attempts++
		n := attempts
		mu.Unlock()
		if n < 2 {
			return nil, errors.New("transient failure")
		}
		return &Handoff{Summary: "ok"}, nil
	}

	if err := m.Run(context.Background(), workerFn); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("expected 2 attempts, got %d: %v", len(seen), seen)
	}
	if seen[0] == seen[1] {
		t.Errorf("branch must differ per attempt, both %q", seen[0])
	}
	if want := "hawk-mission/" + m.ID + "/f1/attempt-1"; seen[0] != want {
		t.Errorf("attempt 1 branch = %q, want %q", seen[0], want)
	}
	if want := "hawk-mission/" + m.ID + "/f1/attempt-2"; seen[1] != want {
		t.Errorf("attempt 2 branch = %q, want %q", seen[1], want)
	}
	if m.Features[0].Branch != seen[1] {
		t.Errorf("final feature branch = %q, want last attempt %q", m.Features[0].Branch, seen[1])
	}
	if m.Features[0].Status != FeatureCompleted {
		t.Errorf("feature should complete after retry, got %s", m.Features[0].Status)
	}
}

func TestMission_Summary(t *testing.T) {
	m := New("test", Config{})
	m.Features = []Feature{
		{Status: FeatureCompleted},
		{Status: FeatureCompleted},
		{Status: FeatureFailed},
	}
	m.CompletedAt = m.StartedAt.Add(5 * time.Second)

	s := m.Summary()
	if s == "" {
		t.Error("expected non-empty summary")
	}
}
