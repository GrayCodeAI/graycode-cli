package safety

import "testing"

func TestSpecWorkflowTransitionsInOrder(t *testing.T) {
	w := SpecWorkflow{}
	for _, tc := range []struct {
		name  string
		slug  string
		stage SpecStage
	}{
		{"Proposal", "demo", SpecStageProposal},
		{"Specify", "", SpecStageSpecify},
		{"Design", "", SpecStageDesign},
		{"Plan", "", SpecStagePlan},
		{"Tasks", "", SpecStageTasks},
		{"ApproveImplementation", "", SpecStageImplementing},
	} {
		if err := w.Transition(tc.name, tc.slug); err != nil {
			t.Fatalf("Transition(%q): %v", tc.name, err)
		}
		if w.Stage != tc.stage {
			t.Fatalf("after %q stage = %v, want %v", tc.name, w.Stage, tc.stage)
		}
	}
}

func TestSpecWorkflowParallelStages(t *testing.T) {
	w := SpecWorkflow{}
	if err := w.Transition("Proposal", "demo"); err != nil {
		t.Fatalf("Proposal: %v", err)
	}
	if err := w.Transition("Design", ""); err != nil {
		t.Fatalf("Design before Specify: %v", err)
	}
	if err := w.Transition("Specify", ""); err != nil {
		t.Fatalf("Specify after Design: %v", err)
	}
	if w.Stage != SpecStageSpecify {
		t.Fatalf("expected Specify stage, got %v", w.Stage)
	}
	if err := w.Transition("Plan", ""); err != nil {
		t.Fatalf("Plan after both Specify and Design: %v", err)
	}
}

func TestSpecWorkflowRejectsInvalidTransitionWithoutMutation(t *testing.T) {
	w := SpecWorkflow{Stage: SpecStageProposal, Slug: "demo", Done: doneProposal}
	if err := w.Transition("Specify", ""); err != nil {
		t.Fatalf("Specify after Proposal: %v", err)
	}
	if err := w.Transition("Plan", ""); err == nil {
		t.Fatal("expected Plan before Design to fail")
	}
	if w.Stage != SpecStageSpecify || w.Slug != "demo" {
		t.Fatalf("invalid transition mutated workflow: %#v", w)
	}
}

func TestSpecWorkflowResetClearsSlugAndStage(t *testing.T) {
	w := SpecWorkflow{Stage: SpecStageImplementing, Slug: "demo"}
	w.Reset()
	if w.Stage != SpecStageNone || w.Slug != "" {
		t.Fatalf("reset result = %#v", w)
	}
}
