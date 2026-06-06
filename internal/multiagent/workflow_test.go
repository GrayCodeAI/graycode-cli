package mission

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// TestWorkflowResumesFromLastStep simulates a crash on step 2, then reconstructs
// the workflow from its persisted state and verifies it resumes from the failed
// step (not from the beginning) and runs to completion.
func TestWorkflowResumesFromLastStep(t *testing.T) {
	dir := t.TempDir()

	// runs[name] counts how many times each step's executor was invoked across
	// both the initial (failing) run and the resumed run.
	runs := map[string]int{}

	defs := func(failOnStep2 bool) []StepDef {
		return []StepDef{
			{Name: "step1", Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
				runs["step1"]++
				st.Values["s1"] = "done"
				return json.RawMessage(`"s1-out"`), nil
			}},
			{Name: "step2", Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
				runs["step2"]++
				if failOnStep2 {
					return nil, errors.New("simulated crash")
				}
				return json.RawMessage(`"s2-out"`), nil
			}},
			{Name: "step3", Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
				runs["step3"]++
				return nil, nil
			}},
		}
	}

	// First run: step2 fails.
	wf := NewWorkflow("deploy", dir, defs(true)...)
	err := wf.Run(context.Background())
	if err == nil {
		t.Fatal("expected first run to fail at step2")
	}
	if runs["step1"] != 1 {
		t.Fatalf("step1 should have run once, ran %d times", runs["step1"])
	}
	if runs["step3"] != 0 {
		t.Fatalf("step3 must not run after step2 failed, ran %d times", runs["step3"])
	}
	if got := wf.LastCompletedStep(); got != "step1" {
		t.Fatalf("expected last completed step1, got %q", got)
	}

	// Reload from disk (simulating restart) with a non-failing step2.
	wf2, err := LoadWorkflow(dir, defs(false)...)
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}
	// The persisted shared value from step1 must survive the restart.
	if wf2.State().Values["s1"] != "done" {
		t.Fatalf("expected persisted value s1=done, got %q", wf2.State().Values["s1"])
	}

	if err := wf2.Resume(context.Background()); err != nil {
		t.Fatalf("resume should succeed, got %v", err)
	}

	// step1 must NOT have re-run; step2 retried once; step3 ran once.
	if runs["step1"] != 1 {
		t.Errorf("step1 re-ran on resume (count=%d); resume must skip completed steps", runs["step1"])
	}
	if runs["step2"] != 2 {
		t.Errorf("step2 should have run twice (fail + resume), got %d", runs["step2"])
	}
	if runs["step3"] != 1 {
		t.Errorf("step3 should have run once, got %d", runs["step3"])
	}
	if !wf2.Done() {
		t.Error("workflow should be Done after resume")
	}
	// step2's output should be persisted from the successful resume.
	for _, s := range wf2.State().Steps {
		if s.Name == "step2" && string(s.Output) != `"s2-out"` {
			t.Errorf("step2 output not persisted: %q", s.Output)
		}
	}
}

// TestWorkflowHumanGate verifies the workflow halts at an un-approved human
// gate, persists that state, and proceeds only after Approve.
func TestWorkflowHumanGate(t *testing.T) {
	dir := t.TempDir()
	ran := map[string]bool{}

	defs := []StepDef{
		{Name: "build", Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
			ran["build"] = true
			return nil, nil
		}},
		{Name: "approve-deploy", HumanGate: true, Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
			ran["approve-deploy"] = true
			return nil, nil
		}},
		{Name: "deploy", Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
			ran["deploy"] = true
			return nil, nil
		}},
	}

	wf := NewWorkflow("gated", dir, defs...)
	err := wf.Run(context.Background())
	if !errors.Is(err, ErrAwaitingApproval) {
		t.Fatalf("expected ErrAwaitingApproval at gate, got %v", err)
	}
	if !ran["build"] {
		t.Error("build should have run before the gate")
	}
	if ran["deploy"] {
		t.Error("deploy must NOT run before the gate is approved")
	}

	// Reload and approve; deploy should then proceed.
	wf2, err := LoadWorkflow(dir, defs...)
	if err != nil {
		t.Fatalf("LoadWorkflow: %v", err)
	}
	if err := wf2.Approve("approve-deploy"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := wf2.Resume(context.Background()); err != nil {
		t.Fatalf("resume after approval: %v", err)
	}
	if !ran["approve-deploy"] || !ran["deploy"] {
		t.Errorf("gate + deploy should run after approval: %+v", ran)
	}
	if !wf2.Done() {
		t.Error("workflow should be Done after approval")
	}
}

// TestWorkflowReject verifies a rejected gate halts the workflow.
func TestWorkflowReject(t *testing.T) {
	dir := t.TempDir()
	defs := []StepDef{
		{Name: "gate", HumanGate: true, Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
			return nil, nil
		}},
		{Name: "after", Fn: func(ctx context.Context, st *WorkflowState) (json.RawMessage, error) {
			t.Error("step after a rejected gate must not run")
			return nil, nil
		}},
	}
	wf := NewWorkflow("rej", dir, defs...)
	if err := wf.Run(context.Background()); !errors.Is(err, ErrAwaitingApproval) {
		t.Fatalf("expected gate to await approval, got %v", err)
	}
	if err := wf.Reject("gate"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if err := wf.Resume(context.Background()); err == nil {
		t.Fatal("expected error resuming a rejected workflow")
	}
}
