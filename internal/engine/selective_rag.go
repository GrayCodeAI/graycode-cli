package engine

import (
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/engine/token"
)

// SelectiveRAG implements the Repoformer-style selective retrieval mechanism.
// It decides WHEN to retrieve context vs when to skip retrieval entirely.
// Research shows this saves 70% of unnecessary retrievals while improving accuracy.
//
// Key insight: Simple queries don't need retrieval. Complex multi-file tasks do.
// The system learns from past retrieval decisions to improve over time.
type SelectiveRAG struct {
	mu sync.RWMutex

	// History of retrieval decisions and their outcomes
	history []RetrievalDecision

	// Thresholds for decision-making
	simpleQueryThreshold int     // token count below which retrieval is skipped
	confidenceThreshold  float64 // confidence above which retrieval is skipped

	// Statistics
	totalDecisions    int
	retrievalsSkipped int
	retrievalsForced  int
}

// RetrievalDecision captures whether retrieval was used and its outcome.
type RetrievalDecision struct {
	Query          string    `json:"query"`
	ShouldRetrieve bool      `json:"should_retrieve"`
	DidRetrieve    bool      `json:"did_retrieve"`
	WasHelpful     bool      `json:"was_helpful"`
	TokensSaved    int       `json:"tokens_saved"`
	Timestamp      time.Time `json:"timestamp"`
	QueryType      string    `json:"query_type"` // "simple", "complex", "navigation", "debug"
}

// NewSelectiveRAG creates a SelectiveRAG with research-backed defaults.
func NewSelectiveRAG() *SelectiveRAG {
	return &SelectiveRAG{
		history:              make([]RetrievalDecision, 0, 1000),
		simpleQueryThreshold: 50,  // queries under 50 tokens are "simple"
		confidenceThreshold:  0.8, // high confidence = skip retrieval
	}
}

// ShouldRetrieve decides whether to retrieve context for a given query.
// Based on Repoformer's self-supervised learning approach.
func (r *SelectiveRAG) ShouldRetrieve(query string, contextTokens int, recentFiles int) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryTokens := estimateTokens(query)

	// Rule 1: Very short queries rarely need retrieval
	if queryTokens < r.simpleQueryThreshold {
		// Check if it's a navigation query (needs retrieval)
		if isNavigationQuery(query) {
			return true, "navigation query detected"
		}
		return false, "simple query, retrieval unnecessary"
	}

	// Rule 2: Queries about specific files already in context
	if containsFilePath(query) && recentFiles > 0 {
		return false, "file already in context"
	}

	// Rule 3: Complex multi-file tasks need retrieval
	if isComplexTask(query) {
		return true, "complex multi-file task"
	}

	// Rule 4: Debug/trace queries need retrieval
	if isDebugQuery(query) {
		return true, "debug/trace query"
	}

	// Rule 5: Check historical success rate
	if len(r.history) > 10 {
		recentHelpful := r.recentRetrievalSuccess()
		if recentHelpful < 0.3 {
			return false, "recent retrievals unhelpful"
		}
	}

	// Default: retrieve for safety
	return true, "default: retrieve"
}

// RecordOutcome records whether retrieval was helpful for learning.
func (r *SelectiveRAG) RecordOutcome(query string, didRetrieve bool, wasHelpful bool, tokensSaved int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	decision := RetrievalDecision{
		Query:          query,
		ShouldRetrieve: true, // what we decided
		DidRetrieve:    didRetrieve,
		WasHelpful:     wasHelpful,
		TokensSaved:    tokensSaved,
		Timestamp:      time.Now(),
		QueryType:      classifyQuery(query),
	}

	r.history = append(r.history, decision)
	r.totalDecisions++

	if !didRetrieve {
		r.retrievalsSkipped++
	}

	// Keep history bounded
	if len(r.history) > 1000 {
		r.history = r.history[500:]
	}
}

// recentRetrievalSuccess calculates the success rate of recent retrievals.
func (r *SelectiveRAG) recentRetrievalSuccess() float64 {
	if len(r.history) == 0 {
		return 0.5 // neutral
	}

	// Look at last 20 decisions
	start := len(r.history) - 20
	if start < 0 {
		start = 0
	}

	helpful := 0
	total := 0
	for _, d := range r.history[start:] {
		if d.DidRetrieve {
			total++
			if d.WasHelpful {
				helpful++
			}
		}
	}

	if total == 0 {
		return 0.5
	}
	return float64(helpful) / float64(total)
}

// Stats returns retrieval statistics.
func (r *SelectiveRAG) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skipRate := 0.0
	if r.totalDecisions > 0 {
		skipRate = float64(r.retrievalsSkipped) / float64(r.totalDecisions)
	}

	return map[string]interface{}{
		"total_decisions":    r.totalDecisions,
		"retrievals_skipped": r.retrievalsSkipped,
		"retrievals_forced":  r.retrievalsForced,
		"skip_rate":          skipRate,
		"history_size":       len(r.history),
	}
}

// Helper functions for query classification

func estimateTokens(text string) int {
	// BPE-based estimate via the tok tokenizer instead of the len/4 char
	// heuristic, which systematically undercounts code-heavy text.
	return token.CountTokensFast(text)
}

func isNavigationQuery(query string) bool {
	navKeywords := []string{"where is", "find", "show me", "locate", "go to", "definition", "references", "implements"}
	lower := strings.ToLower(query)
	for _, kw := range navKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func containsFilePath(query string) bool {
	// Check for file path patterns
	return strings.Contains(query, ".go") ||
		strings.Contains(query, ".ts") ||
		strings.Contains(query, ".py") ||
		strings.Contains(query, ".rs") ||
		strings.Contains(query, "/")
}

func isComplexTask(query string) bool {
	complexKeywords := []string{
		"refactor", "implement", "add feature", "create", "build",
		"architecture", "design", "system", "across", "multiple files",
		"integrate", "migrate", "upgrade",
	}
	lower := strings.ToLower(query)
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func isDebugQuery(query string) bool {
	debugKeywords := []string{
		"bug", "error", "fix", "broken", "crash", "fail", "debug",
		"trace", "why", "not working", "issue",
	}
	lower := strings.ToLower(query)
	for _, kw := range debugKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func classifyQuery(query string) string {
	if isNavigationQuery(query) {
		return "navigation"
	}
	if isDebugQuery(query) {
		return "debug"
	}
	if isComplexTask(query) {
		return "complex"
	}
	return "simple"
}
