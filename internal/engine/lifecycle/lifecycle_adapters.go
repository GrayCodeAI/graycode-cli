package lifecycle

import (
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
)

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
type SkillDistillerAdapter struct {
	SD     *memory.SkillDistiller
	Chat   func(prompt string) (string, error)
	Store  func(skill *memory.DistilledSkill) error
	Search func(query string) ([]*memory.DistilledSkill, error)
}

func (a *SkillDistillerAdapter) Distill(goal string, steps []string, outcome string) error {
	if a.SD == nil {
		return nil
	}
	if a.Chat == nil {
		return fmt.Errorf("skill distiller: chat function is not configured")
	}
	if a.Store == nil {
		return fmt.Errorf("skill distiller: store function is not configured")
	}
	response, err := a.Chat(a.SD.BuildSkillPrompt(goal, steps, nil, outcome))
	if err != nil {
		return fmt.Errorf("skill distiller: extract skill: %w", err)
	}
	skill, err := a.SD.ParseSkill(response)
	if err != nil {
		return fmt.Errorf("skill distiller: parse skill: %w", err)
	}
	if err := a.Store(skill); err != nil {
		return fmt.Errorf("skill distiller: persist skill: %w", err)
	}
	return nil
}

func (a *SkillDistillerAdapter) Retrieve(query string) []string {
	if a == nil || a.Search == nil {
		return nil
	}
	skills, err := a.Search(query)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if skill == nil {
			continue
		}
		out = append(out, skill.Name+": "+skill.Description)
	}
	return out
}
