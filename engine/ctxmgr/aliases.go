// Package ctxmgr is the Stage-1 namespace for context budget, decay, packing,
// providers, visualisation, and read-only context. See ../REFACTOR_PLAN.md.
//
// Named "ctxmgr" (not "context") to avoid shadowing the stdlib context package.
package ctxmgr

import (
	"time"

	"github.com/GrayCodeAI/hawk/engine"
)

type ContextBudget = engine.ContextBudget
type ContextAllocation = engine.ContextAllocation
type ContextDecay = engine.ContextDecay
type DecayEntry = engine.DecayEntry
type DecayStats = engine.DecayStats
type PackingStrategy = engine.PackingStrategy
type ContextPacker = engine.ContextPacker
type ScoredMessage = engine.ScoredMessage
type PackingResult = engine.PackingResult
type ContextProvider = engine.ContextProvider
type ContextItem = engine.ContextItem
type ContextManager = engine.ContextManager
type GitContextProvider = engine.GitContextProvider
type FileContextProvider = engine.FileContextProvider
type ErrorContextProvider = engine.ErrorContextProvider
type DependencyContextProvider = engine.DependencyContextProvider
type ContextVisualizer = engine.ContextVisualizer
type ContextSection = engine.ContextSection
type VizContextItem = engine.VizContextItem
type ContextSnapshot = engine.ContextSnapshot
type ReadOnlyContext = engine.ReadOnlyContext
type ContextFile = engine.ContextFile
type ContextFileOption = engine.ContextFileOption
type ContextStats = engine.ContextStats

func NewContextBudget(contextSize int) *ContextBudget { return engine.NewContextBudget(contextSize) }
func NewContextDecay(halfLife time.Duration) *ContextDecay { return engine.NewContextDecay(halfLife) }
func NewContextPacker(maxTokens int) *ContextPacker   { return engine.NewContextPacker(maxTokens) }
func NewContextManager(budget int) *ContextManager    { return engine.NewContextManager(budget) }
func NewContextVisualizer(max int) *ContextVisualizer { return engine.NewContextVisualizer(max) }
func NewReadOnlyContext(maxBudget int) *ReadOnlyContext { return engine.NewReadOnlyContext(maxBudget) }
func FormatContextItems(items []ContextItem) string   { return engine.FormatContextItems(items) }
func PrioritizeItems(items []ContextItem, budget int) []ContextItem {
	return engine.PrioritizeItems(items, budget)
}
func SuggestFiles(projectDir string) []string { return engine.SuggestFiles(projectDir) }
func WithPinned() ContextFileOption           { return engine.WithPinned() }
func WithAutoRefresh() ContextFileOption      { return engine.WithAutoRefresh() }
