// Package memory is the Stage-1 namespace for knowledge, experience, and
// memory consolidation types. See ../REFACTOR_PLAN.md.
package memory

import "github.com/GrayCodeAI/hawk/engine"

type KnowledgeEntry = engine.KnowledgeEntry
type KnowledgeStats = engine.KnowledgeStats
type KnowledgeBase = engine.KnowledgeBase
type Experience = engine.Experience
type ExperienceStats = engine.ExperienceStats
type ExperienceStore = engine.ExperienceStore
type RawMemory = engine.RawMemory
type ConsolidatedMemory = engine.ConsolidatedMemory
type ConsolidatorStats = engine.ConsolidatorStats
type MemoryConsolidator = engine.MemoryConsolidator

func NewKnowledgeBase(dir string) *KnowledgeBase       { return engine.NewKnowledgeBase(dir) }
func NewExperienceStore(dir string) *ExperienceStore   { return engine.NewExperienceStore(dir) }
func NewMemoryConsolidator(dir string) *MemoryConsolidator { return engine.NewMemoryConsolidator(dir) }
