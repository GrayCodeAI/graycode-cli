package safety

import "fmt"

// SpecWorkflow is the validated state machine for spec-driven development.
// It intentionally contains no filesystem or UI concerns.
type SpecWorkflow struct {
	Stage SpecStage
	Slug  string
	Done  specDone // tracks which parallel stages are complete
}

// specDone is a bitmask of completed parallel stages.
type specDone int

const (
	doneProposal specDone = 1 << iota
	doneSpecify
	doneDesign
)

// Transition validates and applies a successful workflow tool transition.
// The state is changed only after all preconditions pass.
// Workflow: Proposal → Specify ↘
//
//	Design → Plan → Tasks → ApproveImplementation → Implementing
//
// Specify and Design can run in parallel after Proposal.
func (w *SpecWorkflow) Transition(toolName, slug string) error {
	switch canonicalToolName(toolName) {
	case "Proposal":
		if w.Stage != SpecStageNone && w.Stage != SpecStageProposal {
			return fmt.Errorf("proposal is unavailable at spec stage %d", w.Stage)
		}
		if slug == "" {
			return fmt.Errorf("proposal requires a non-empty spec slug")
		}
		w.Slug = slug
		w.Stage = SpecStageProposal
		w.Done |= doneProposal
	case "Specify":
		if w.Stage == SpecStageNone {
			return fmt.Errorf("specify requires a completed proposal stage")
		}
		if w.Stage == SpecStageTasks || w.Stage == SpecStageImplementing {
			return fmt.Errorf("specify is unavailable at spec stage %d", w.Stage)
		}
		if w.Slug == "" && slug != "" {
			w.Slug = slug
		}
		w.Stage = SpecStageSpecify
		w.Done |= doneSpecify
	case "Design":
		if w.Stage == SpecStageNone {
			return fmt.Errorf("design requires a completed proposal stage")
		}
		if w.Stage == SpecStageTasks || w.Stage == SpecStageImplementing {
			return fmt.Errorf("design is unavailable at spec stage %d", w.Stage)
		}
		w.Stage = SpecStageDesign
		w.Done |= doneDesign
	case "Plan":
		if w.Slug == "" || w.Done&(doneSpecify|doneDesign) != doneSpecify|doneDesign {
			return fmt.Errorf("plan requires completed specify and design stages")
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
	w.Done = 0
}
