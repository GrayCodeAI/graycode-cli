// Package memory is the Stage-1 namespace for knowledge, experience, and
// memory consolidation types. See ../REFACTOR_PLAN.md.
package memory

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	KnowledgeEntry     = engine.KnowledgeEntry
	KnowledgeStats     = engine.KnowledgeStats
	KnowledgeBase      = engine.KnowledgeBase
	Experience         = engine.Experience
	ExperienceStats    = engine.ExperienceStats
	ExperienceStore    = engine.ExperienceStore
	RawMemory          = engine.RawMemory
	ConsolidatedMemory = engine.ConsolidatedMemory
	ConsolidatorStats  = engine.ConsolidatorStats
	MemoryConsolidator = engine.MemoryConsolidator
)

func NewKnowledgeBase(dir string) *KnowledgeBase           { return engine.NewKnowledgeBase(dir) }
func NewExperienceStore(dir string) *ExperienceStore       { return engine.NewExperienceStore(dir) }
func NewMemoryConsolidator(dir string) *MemoryConsolidator { return engine.NewMemoryConsolidator(dir) }
