package engine

import "github.com/GrayCodeAI/hawk/memory"

// EvolvingMemoryAdapter bridges memory.EvolvingMemory to the EvolvingMemoryInterface.
type EvolvingMemoryAdapter struct {
	EM *memory.EvolvingMemory
}

func (a *EvolvingMemoryAdapter) Learn(pattern, lesson string) error {
	if a.EM == nil {
		return nil
	}
	a.EM.Learn(pattern, lesson, "session_lifecycle")
	return nil
}

func (a *EvolvingMemoryAdapter) Retrieve(query string) []string {
	if a.EM == nil {
		return nil
	}
	guidelines := a.EM.Retrieve(query, 5)
	var out []string
	for _, g := range guidelines {
		out = append(out, g.Lesson)
	}
	return out
}

func (a *EvolvingMemoryAdapter) Format() string {
	if a.EM == nil {
		return ""
	}
	return a.EM.Format(5)
}

// SkillDistillerAdapter bridges memory.SkillDistiller to SkillStoreInterface.
// Skill distillation builds a prompt for LLM extraction — the actual distilled
// skills are stored as files in hawk-skills/.
type SkillDistillerAdapter struct {
	SD *memory.SkillDistiller
}

func (a *SkillDistillerAdapter) Distill(goal string, steps []string, outcome string) error {
	if a.SD == nil {
		return nil
	}
	// Build the prompt that would be sent to an LLM for skill extraction.
	// In a full implementation, this would call the LLM and persist the result.
	_ = a.SD.BuildSkillPrompt(goal, steps, nil, outcome)
	return nil
}

func (a *SkillDistillerAdapter) Retrieve(_ string) []string {
	return nil
}
