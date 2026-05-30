package mission

import (
	"context"
	"errors"
	"testing"
)

func testFeatures() []Feature {
	return []Feature{
		{ID: "feat-1", Description: "do A", ExpectedBehavior: "A works", Status: FeaturePending},
		{ID: "feat-2", Description: "do B", ExpectedBehavior: "B works", Status: FeaturePending},
	}
}

func TestNew_DefaultMaxFixAttempts(t *testing.T) {
	m := New("prompt", Config{})
	if m.Config.MaxFixAttempts != 3 {
		t.Errorf("expected default MaxFixAttempts=3, got %d", m.Config.MaxFixAttempts)
	}
}

func TestGeneratePRD(t *testing.T) {
	m := New("prompt", Config{})
	m.Features = testFeatures()

	err := m.GeneratePRD(context.Background(), func(ctx context.Context, f *Feature) (string, error) {
		return "PRD for " + f.ID, nil
	})
	if err != nil {
		t.Fatalf("GeneratePRD: %v", err)
	}
	for _, f := range m.Features {
		if f.PRD == "" {
			t.Errorf("feature %s missing PRD", f.ID)
		}
	}
	if m.Status != StatusPlanningPRD {
		t.Errorf("expected status %q, got %q", StatusPlanningPRD, m.Status)
	}
}

func TestVerify_MarksFailed(t *testing.T) {
	m := New("prompt", Config{})
	m.Features = testFeatures()
	m.Features[0].Status = FeatureCompleted
	m.Features[1].Status = FeatureCompleted

	err := m.Verify(context.Background(), func(ctx context.Context, f *Feature, h *Handoff) (bool, string, error) {
		if f.ID == "feat-2" {
			return false, "B does not work", nil
		}
		return true, "ok", nil
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if m.Features[0].Status != FeatureCompleted {
		t.Errorf("feat-1 should stay completed, got %s", m.Features[0].Status)
	}
	if m.Features[1].Status != FeatureFailed {
		t.Errorf("feat-2 should be failed, got %s", m.Features[1].Status)
	}
	if m.Features[1].VerificationResult != "B does not work" {
		t.Errorf("verification result not recorded: %q", m.Features[1].VerificationResult)
	}
}

func TestFix_Loop(t *testing.T) {
	m := New("prompt", Config{MaxFixAttempts: 3})
	m.Features = testFeatures()
	m.Features[1].Status = FeatureFailed
	m.Features[1].VerificationResult = "broken"

	attempts := 0
	err := m.Fix(context.Background(), func(ctx context.Context, f *Feature, vr string, h *Handoff) (*Handoff, error) {
		attempts++
		if attempts < 2 {
			return nil, errors.New("still broken")
		}
		return &Handoff{Summary: "fixed"}, nil
	})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if m.Features[1].Status != FeatureCompleted {
		t.Errorf("expected feat-2 fixed to completed, got %s", m.Features[1].Status)
	}
	if m.Features[1].FixAttempts != 2 {
		t.Errorf("expected 2 fix attempts, got %d", m.Features[1].FixAttempts)
	}
}

func TestFix_ExhaustsAttempts(t *testing.T) {
	m := New("prompt", Config{MaxFixAttempts: 2})
	m.Features = testFeatures()
	m.Features[0].Status = FeatureFailed
	m.Features[0].VerificationResult = "broken"

	err := m.Fix(context.Background(), func(ctx context.Context, f *Feature, vr string, h *Handoff) (*Handoff, error) {
		return nil, errors.New("cannot fix")
	})
	if err != nil {
		t.Fatalf("Fix: %v", err)
	}
	if m.Features[0].Status != FeatureFailed {
		t.Errorf("expected feat-1 to remain failed, got %s", m.Features[0].Status)
	}
	if m.Features[0].FixAttempts != 2 {
		t.Errorf("expected attempts capped at 2, got %d", m.Features[0].FixAttempts)
	}
}

func TestRunStaged_FullPipeline(t *testing.T) {
	m := New("prompt", Config{
		MaxWorkers:        2,
		EnablePRDPhase:    true,
		EnableVerifyPhase: true,
		MaxFixAttempts:    2,
	})
	m.Features = testFeatures()

	prdCalls := 0
	worker := func(ctx context.Context, f *Feature, dir string, cfg Config) (*Handoff, error) {
		return &Handoff{Summary: "done " + f.ID, TestsPassed: true}, nil
	}

	err := m.RunStaged(
		context.Background(), worker,
		WithPRD(func(ctx context.Context, f *Feature) (string, error) {
			prdCalls++
			return "prd", nil
		}),
		WithVerify(func(ctx context.Context, f *Feature, h *Handoff) (bool, string, error) {
			return true, "verified", nil
		}),
	)
	if err != nil {
		t.Fatalf("RunStaged: %v", err)
	}
	if prdCalls != 2 {
		t.Errorf("expected PRD generated for 2 features, got %d", prdCalls)
	}
	if m.Status != StatusCompleted {
		t.Errorf("expected completed, got %q", m.Status)
	}
}

func TestRunStaged_BackwardCompat(t *testing.T) {
	// Without enabling PRD/verify phases, RunStaged should behave like Run.
	m := New("prompt", Config{MaxWorkers: 2})
	m.Features = testFeatures()

	worker := func(ctx context.Context, f *Feature, dir string, cfg Config) (*Handoff, error) {
		return &Handoff{Summary: "done", TestsPassed: true}, nil
	}
	if err := m.RunStaged(context.Background(), worker); err != nil {
		t.Fatalf("RunStaged: %v", err)
	}
	if m.Status != StatusCompleted {
		t.Errorf("expected completed, got %q", m.Status)
	}
}
