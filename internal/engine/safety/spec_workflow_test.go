package safety

import "testing"

func TestSpecWorkflowTransitionsInOrder(t *testing.T) {
	w := SpecWorkflow{}
	for _, tc := range []struct {
		name  string
		slug  string
		stage SpecStage
	}{
		{"Specify", "demo", SpecStageSpecify},
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

func TestSpecWorkflowRejectsInvalidTransitionWithoutMutation(t *testing.T) {
	w := SpecWorkflow{Stage: SpecStageSpecify, Slug: "demo"}
	if err := w.Transition("Tasks", ""); err == nil {
		t.Fatal("expected Tasks before Plan to fail")
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
