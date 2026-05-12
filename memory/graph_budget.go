package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	yaadEngine "github.com/GrayCodeAI/yaad/engine"
	"github.com/GrayCodeAI/yaad/storage"
)

// GraphAwareBudget makes memory allocation smarter by using yaad's graph
// to determine what context to inject. It considers:
// - Active file subgraphs (what memories relate to files being worked on)
// - Confidence and staleness (prioritize high-confidence, non-stale memories)
// - Task complexity (more memory for complex multi-file tasks)
// - Impact analysis (what's affected by current changes)
type GraphAwareBudget struct {
	bridge      *YaadBridge
	proactive   *ProactiveContext
	mu          sync.Mutex
	baseBudget  int
	maxBudget   int
}

// MemoryInjection represents prioritized memory content for prompt injection.
type MemoryInjection struct {
	Content     string
	TokenCount  int
	Source      string // "graph", "global", "proactive", "file"
	Priority    float64
}

// NewGraphAwareBudget creates a graph-aware memory budget allocator.
func NewGraphAwareBudget(bridge *YaadBridge, proactive *ProactiveContext) *GraphAwareBudget {
	return &GraphAwareBudget{
		bridge:     bridge,
		proactive:  proactive,
		baseBudget: 1000,
		maxBudget:  4000,
	}
}

// ComputeBudget determines how many tokens to allocate for memory
// based on task complexity and graph density.
func (gb *GraphAwareBudget) ComputeBudget(activeFiles []string, taskComplexity float64) int {
	if !gb.bridge.Ready() {
		return gb.baseBudget
	}

	// Base budget scales with complexity
	budget := gb.baseBudget + int(float64(gb.maxBudget-gb.baseBudget)*taskComplexity)

	// If many files are active, allocate more memory budget
	if len(activeFiles) > 3 {
		budget += 500
	}

	if budget > gb.maxBudget {
		budget = gb.maxBudget
	}
	return budget
}

// BuildInjection constructs the optimal memory context within the given budget.
// It prioritizes by: pinned > high-confidence > file-relevant > recent.
func (gb *GraphAwareBudget) BuildInjection(query string, activeFiles []string, budget int) string {
	if !gb.bridge.Ready() {
		return ""
	}
	gb.mu.Lock()
	defer gb.mu.Unlock()

	var sections []MemoryInjection

	// 1. Pinned memories (always included, highest priority)
	pinned := gb.getPinnedMemories()
	if pinned != "" {
		sections = append(sections, MemoryInjection{
			Content:  pinned,
			Source:   "pinned",
			Priority: 1.0,
		})
	}

	// 2. High-confidence conventions (core project knowledge)
	conventions := gb.getHighConfidenceConventions()
	if conventions != "" {
		sections = append(sections, MemoryInjection{
			Content:  conventions,
			Source:   "conventions",
			Priority: 0.9,
		})
	}

	// 3. File-relevant memories (based on active files)
	if len(activeFiles) > 0 && gb.proactive != nil {
		fileCtx := gb.proactive.ContextForActiveFiles(budget / 4)
		if fileCtx != "" {
			sections = append(sections, MemoryInjection{
				Content:  fileCtx,
				Source:   "file",
				Priority: 0.8,
			})
		}
	}

	// 4. Query-relevant memories (graph-traversed)
	if query != "" {
		queryCtx := gb.getQueryRelevant(query, budget/4)
		if queryCtx != "" {
			sections = append(sections, MemoryInjection{
				Content:  queryCtx,
				Source:   "graph",
				Priority: 0.7,
			})
		}
	}

	// 5. Active tasks (what's in progress)
	tasks := gb.getActiveTasks()
	if tasks != "" {
		sections = append(sections, MemoryInjection{
			Content:  tasks,
			Source:   "tasks",
			Priority: 0.6,
		})
	}

	// Sort by priority and fit within budget
	sortInjections(sections)
	return gb.fitToBudget(sections, budget)
}

func (gb *GraphAwareBudget) getPinnedMemories() string {
	pinned := true
	nodes, err := gb.bridge.store.ListNodes(context.Background(), storage.NodeFilter{
		Pinned: &pinned,
		Limit:  10,
	})
	if err != nil || len(nodes) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Pinned\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("- %s\n", n.Content))
	}
	return sb.String()
}

func (gb *GraphAwareBudget) getHighConfidenceConventions() string {
	nodes, err := gb.bridge.store.ListNodes(context.Background(), storage.NodeFilter{
		Type:          "convention",
		MinConfidence: 0.7,
		Limit:         10,
	})
	if err != nil || len(nodes) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Conventions\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("- %s\n", n.Content))
	}
	return sb.String()
}

func (gb *GraphAwareBudget) getQueryRelevant(query string, budget int) string {
	result, err := gb.bridge.engine.Recall(context.Background(), yaadEngine.RecallOpts{
		Query:  query,
		Budget: budget,
		Limit:  5,
		Depth:  2,
	})
	if err != nil || result == nil || len(result.Nodes) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, n := range result.Nodes {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", n.Type, n.Content))
	}
	return sb.String()
}

func (gb *GraphAwareBudget) getActiveTasks() string {
	nodes, err := gb.bridge.store.ListNodes(context.Background(), storage.NodeFilter{
		Type:          "task",
		MinConfidence: 0.3,
		Limit:         5,
	})
	if err != nil || len(nodes) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### Active Tasks\n")
	for _, n := range nodes {
		sb.WriteString(fmt.Sprintf("- %s\n", n.Content))
	}
	return sb.String()
}

func (gb *GraphAwareBudget) fitToBudget(sections []MemoryInjection, budget int) string {
	var sb strings.Builder
	tokensUsed := 0

	for _, s := range sections {
		sectionTokens := len(s.Content) / 4 // rough estimate
		if tokensUsed+sectionTokens > budget {
			// Try to fit a truncated version
			remaining := budget - tokensUsed
			if remaining > 50 {
				maxChars := remaining * 4
				if maxChars < len(s.Content) {
					sb.WriteString(s.Content[:maxChars])
					sb.WriteString("...\n")
				}
			}
			break
		}
		sb.WriteString(s.Content)
		sb.WriteString("\n")
		tokensUsed += sectionTokens
	}

	return sb.String()
}

func sortInjections(sections []MemoryInjection) {
	for i := 0; i < len(sections); i++ {
		for j := i + 1; j < len(sections); j++ {
			if sections[j].Priority > sections[i].Priority {
				sections[i], sections[j] = sections[j], sections[i]
			}
		}
	}
}
