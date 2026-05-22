package engine

import "github.com/GrayCodeAI/hawk/internal/engine/memory"

type KnowledgeEntry = memory.KnowledgeEntry
type KnowledgeStats = memory.KnowledgeStats
type KnowledgeBase = memory.KnowledgeBase
type Experience = memory.Experience
type ExperienceStats = memory.ExperienceStats
type ExperienceStore = memory.ExperienceStore
type RawMemory = memory.RawMemory
type ConsolidatedMemory = memory.ConsolidatedMemory
type ConsolidatorStats = memory.ConsolidatorStats
type MemoryConsolidator = memory.MemoryConsolidator

var NewKnowledgeBase = memory.NewKnowledgeBase
var NewExperienceStore = memory.NewExperienceStore
var NewMemoryConsolidator = memory.NewMemoryConsolidator
