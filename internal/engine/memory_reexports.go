package engine

import "github.com/GrayCodeAI/hawk/internal/engine/memory"

type (
	KnowledgeEntry     = memory.KnowledgeEntry
	KnowledgeStats     = memory.KnowledgeStats
	KnowledgeBase      = memory.KnowledgeBase
	Experience         = memory.Experience
	ExperienceStats    = memory.ExperienceStats
	ExperienceStore    = memory.ExperienceStore
	RawMemory          = memory.RawMemory
	ConsolidatedMemory = memory.ConsolidatedMemory
	ConsolidatorStats  = memory.ConsolidatorStats
	MemoryConsolidator = memory.MemoryConsolidator
)

var (
	NewKnowledgeBase      = memory.NewKnowledgeBase
	NewExperienceStore    = memory.NewExperienceStore
	NewMemoryConsolidator = memory.NewMemoryConsolidator
)
