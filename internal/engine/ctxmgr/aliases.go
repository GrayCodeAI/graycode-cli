// Package ctxmgr is the Stage-1 namespace for context budget, decay, packing,
// providers, visualisation, and read-only context. See ../REFACTOR_PLAN.md.
//
// Named "ctxmgr" (not "context") to avoid shadowing the stdlib context package.
package ctxmgr

import (
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

type (
	ContextBudget             = engine.ContextBudget
	ContextAllocation         = engine.ContextAllocation
	ContextDecay              = engine.ContextDecay
	DecayEntry                = engine.DecayEntry
	DecayStats                = engine.DecayStats
	PackingStrategy           = engine.PackingStrategy
	ContextPacker             = engine.ContextPacker
	ScoredMessage             = engine.ScoredMessage
	PackingResult             = engine.PackingResult
	ContextProvider           = engine.ContextProvider
	ContextItem               = engine.ContextItem
	ContextManager            = engine.ContextManager
	GitContextProvider        = engine.GitContextProvider
	FileContextProvider       = engine.FileContextProvider
	ErrorContextProvider      = engine.ErrorContextProvider
	DependencyContextProvider = engine.DependencyContextProvider
	ContextVisualizer         = engine.ContextVisualizer
	ContextSection            = engine.ContextSection
	VizContextItem            = engine.VizContextItem
	ContextSnapshot           = engine.ContextSnapshot
	ReadOnlyContext           = engine.ReadOnlyContext
	ContextFile               = engine.ContextFile
	ContextFileOption         = engine.ContextFileOption
	ContextStats              = engine.ContextStats
)

func NewContextBudget(contextSize int) *ContextBudget { return engine.NewContextBudget(contextSize) }

func NewContextDecay(halfLife time.Duration) *ContextDecay { return engine.NewContextDecay(halfLife) }

func NewContextPacker(maxTokens int) *ContextPacker { return engine.NewContextPacker(maxTokens) }

func NewContextManager(budget int) *ContextManager { return engine.NewContextManager(budget) }

func NewContextVisualizer(max int) *ContextVisualizer { return engine.NewContextVisualizer(max) }

func NewReadOnlyContext(maxBudget int) *ReadOnlyContext { return engine.NewReadOnlyContext(maxBudget) }

func FormatContextItems(items []ContextItem) string { return engine.FormatContextItems(items) }

func PrioritizeItems(items []ContextItem, budget int) []ContextItem {
	return engine.PrioritizeItems(items, budget)
}
func SuggestFiles(projectDir string) []string { return engine.SuggestFiles(projectDir) }
func WithPinned() ContextFileOption           { return engine.WithPinned() }
func WithAutoRefresh() ContextFileOption      { return engine.WithAutoRefresh() }
