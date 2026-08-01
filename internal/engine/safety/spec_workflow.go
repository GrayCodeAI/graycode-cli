package safety

import "fmt"

// SpecWorkflow is the validated state machine for spec-driven development.
// It intentionally contains no filesystem or UI concerns.
type SpecWorkflow struct {
	Stage SpecStage
	Slug  string
}

// Transition validates and applies a successful workflow tool transition.
// The state is changed only after all preconditions pass.
func (w *SpecWorkflow) Transition(toolName, slug string) error {
	switch canonicalToolName(toolName) {
	case "Specify":
		if w.Stage != SpecStageNone && w.Stage != SpecStageSpecify {
			return fmt.Errorf("specify is unavailable at spec stage %d", w.Stage)
		}
		if slug == "" {
			return fmt.Errorf("specify requires a non-empty spec slug")
		}
		w.Slug = slug
		w.Stage = SpecStageSpecify
	case "Plan":
		if w.Stage != SpecStageSpecify || w.Slug == "" {
			return fmt.Errorf("plan requires a completed specify stage")
		}
		w.Stage = SpecStagePlan
	case "Tasks":
		if w.Stage != SpecStagePlan {
			return fmt.Errorf("tasks requires a completed plan stage")
		}
		w.Stage = SpecStageTasks
	case "ApproveImplementation":
		if w.Stage != SpecStageTasks {
			return fmt.Errorf("ApproveImplementation requires a completed Tasks stage")
		}
		w.Stage = SpecStageImplementing
	default:
		return nil
	}
	return nil
}

// Reset clears all workflow state, including the slug.
func (w *SpecWorkflow) Reset() {
	w.Stage = SpecStageNone
	w.Slug = ""
}
