package engine

import (
	"fmt"
	"strings"
)

// ContextBudget allocates the model's context window across different content categories.
// Based on research: proper allocation prevents context overflow and optimizes
// information density per token spent.
//
// The allocator ensures:
//   - Fixed allocations for system prompt, repo map, memory, workspace context
//   - Adaptive allocation for pre-loaded files (expands when conversation is short)
//   - Managed conversation history (triggers compaction when exceeded)
//   - Reserved budget for model output and safety margin
//
// Research basis: All top coding agents (Claude Code, Cursor, Aider) manage context,
// but none formalize it as an explicit budget with categories. This is the missing
// architectural glue.
type ContextBudget struct {
	Total int // model's full context window

	// Fixed allocations
	SystemPrompt int // 3000-5000 tokens (rules, identity)
	ToolDefs     int // 2000-3000 tokens (tool descriptions)
	RepoMap      int // 2000-4000 tokens (ranked symbol map)
	Memory       int // 1000-2000 tokens (yaad/zenbrain context)
	Workspace    int // 500 tokens (git status, branch, recent commits)

	// Adaptive allocations
	PreloadedFiles int // 10000-30000 tokens (relevant code context)
	Conversation   int // remaining (managed by compaction)

	// Reserved
	OutputReserve int // 16000-20000 tokens (model response space)
	SafetyMargin  int // 10000-15000 tokens (estimation errors, API overhead)
}

// ContextAllocation shows where tokens are going for the current conversation state.
type ContextAllocation struct {
	SystemPrompt   int
	ToolDefs       int
	RepoMap        int
	Memory         int
	Workspace      int
	PreloadedFiles int
	Conversation   int
	OutputReserve  int
	SafetyMargin   int
	Remaining      int // should be ~0 if properly allocated
}

// NewContextBudget creates a budget for the given model context size.
// Allocations scale proportionally with the context window while respecting
// sensible floors and ceilings per category.
func NewContextBudget(contextSize int) *ContextBudget {
	b := &ContextBudget{Total: contextSize}

	// Fixed allocations — scale with context but clamp to documented ranges.
	b.SystemPrompt = clamp(contextSize*3/100, 3000, 5000)   // ~3%
	b.ToolDefs = clamp(contextSize*2/100, 2000, 3000)       // ~2%
	b.RepoMap = clamp(contextSize*2/100, 2000, 4000)        // ~2%
	b.Memory = clamp(contextSize*1/100, 1000, 2000)         // ~1%
	b.Workspace = clamp(contextSize*1/200, 300, 500)        // ~0.5%

	// Reserved — output space and safety margin.
	b.OutputReserve = clamp(contextSize*10/100, 4000, 20000) // ~10%
	b.SafetyMargin = clamp(contextSize*7/100, 2000, 15000)   // ~7%

	// Adaptive: PreloadedFiles gets the maximum initial allocation.
	// Conversation gets whatever remains.
	fixed := b.SystemPrompt + b.ToolDefs + b.RepoMap + b.Memory + b.Workspace
	reserved := b.OutputReserve + b.SafetyMargin
	adaptive := contextSize - fixed - reserved

	// Split adaptive space: PreloadedFiles gets up to 30K, rest is Conversation.
	maxPreload := clamp(contextSize*15/100, 5000, 30000) // ~15%, capped
	if maxPreload > adaptive {
		maxPreload = adaptive / 2
	}
	b.PreloadedFiles = maxPreload
	b.Conversation = adaptive - maxPreload

	return b
}

// Allocate distributes the budget based on current conversation length.
// As conversation grows, PreloadedFiles shrinks to make room.
func (b *ContextBudget) Allocate(conversationTokens int) *ContextAllocation {
	a := &ContextAllocation{
		SystemPrompt:  b.SystemPrompt,
		ToolDefs:      b.ToolDefs,
		RepoMap:       b.RepoMap,
		Memory:        b.Memory,
		Workspace:     b.Workspace,
		OutputReserve: b.OutputReserve,
		SafetyMargin:  b.SafetyMargin,
	}

	fixed := a.SystemPrompt + a.ToolDefs + a.RepoMap + a.Memory + a.Workspace
	reserved := a.OutputReserve + a.SafetyMargin
	adaptive := b.Total - fixed - reserved
	if adaptive < 0 {
		adaptive = 0
	}

	// Compute adaptive file budget based on conversation pressure.
	fileBudget := b.adaptiveFileBudget(conversationTokens, adaptive)
	a.PreloadedFiles = fileBudget
	a.Conversation = adaptive - fileBudget
	if a.Conversation < 0 {
		a.Conversation = 0
	}

	used := fixed + reserved + a.PreloadedFiles + a.Conversation
	a.Remaining = b.Total - used
	return a
}

// ShouldCompact returns true if conversation exceeds its allocation.
func (b *ContextBudget) ShouldCompact(conversationTokens int) bool {
	alloc := b.Allocate(conversationTokens)
	return conversationTokens > alloc.Conversation
}

// FilesBudget returns the current budget available for pre-loaded file context.
func (b *ContextBudget) FilesBudget(conversationTokens int) int {
	alloc := b.Allocate(conversationTokens)
	return alloc.PreloadedFiles
}

// UsageReport returns a human-readable breakdown of current allocation.
func (b *ContextBudget) UsageReport(conversationTokens int) string {
	alloc := b.Allocate(conversationTokens)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Context Budget (%d tokens total)\n", b.Total))
	sb.WriteString(strings.Repeat("-", 44) + "\n")

	lines := []struct {
		label string
		value int
	}{
		{"System Prompt", alloc.SystemPrompt},
		{"Tool Definitions", alloc.ToolDefs},
		{"Repo Map", alloc.RepoMap},
		{"Memory", alloc.Memory},
		{"Workspace", alloc.Workspace},
		{"Preloaded Files", alloc.PreloadedFiles},
		{"Conversation", alloc.Conversation},
		{"Output Reserve", alloc.OutputReserve},
		{"Safety Margin", alloc.SafetyMargin},
	}

	for _, l := range lines {
		pct := 0.0
		if b.Total > 0 {
			pct = float64(l.value) * 100 / float64(b.Total)
		}
		sb.WriteString(fmt.Sprintf("  %-20s %6d  (%4.1f%%)\n", l.label, l.value, pct))
	}

	sb.WriteString(strings.Repeat("-", 44) + "\n")
	sb.WriteString(fmt.Sprintf("  %-20s %6d\n", "Remaining", alloc.Remaining))

	if conversationTokens > 0 {
		sb.WriteString(fmt.Sprintf("\nConversation: %d / %d tokens", conversationTokens, alloc.Conversation))
		if b.ShouldCompact(conversationTokens) {
			sb.WriteString(" [COMPACT NEEDED]")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// adaptiveFileBudget computes the pre-loaded file budget based on how much
// conversation space is consumed. Short conversations get maximum file context;
// long conversations reduce files to the minimum to preserve history.
func (b *ContextBudget) adaptiveFileBudget(conversationTokens, adaptiveSpace int) int {
	minFiles := clamp(b.Total*5/100, 2000, 10000)  // floor: ~5%
	maxFiles := clamp(b.Total*15/100, 5000, 30000)  // ceiling: ~15%

	if maxFiles > adaptiveSpace {
		maxFiles = adaptiveSpace / 2
	}
	if minFiles > maxFiles {
		minFiles = maxFiles
	}

	// Linear ramp-down: at 0 conversation tokens, use maxFiles.
	// At conversationThreshold, use minFiles.
	threshold := adaptiveSpace - minFiles
	if threshold <= 0 {
		return minFiles
	}

	if conversationTokens <= 0 {
		return maxFiles
	}
	if conversationTokens >= threshold {
		return minFiles
	}

	// Linear interpolation between max and min.
	span := maxFiles - minFiles
	ratio := float64(conversationTokens) / float64(threshold)
	budget := maxFiles - int(float64(span)*ratio)
	return clamp(budget, minFiles, maxFiles)
}

// clamp constrains v to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
