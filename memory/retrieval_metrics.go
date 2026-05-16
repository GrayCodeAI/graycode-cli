package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RetrievalMetrics tracks memory retrieval effectiveness in real-time.
// It logs every recall operation, what was returned, and measures hit rate
// to enable auto-tuning of search parameters.
type RetrievalMetrics struct {
	mu        sync.Mutex
	entries   []RetrievalEntry
	hitCount  int
	missCount int
	savePath  string
}

// RetrievalEntry records a single recall operation and its effectiveness.
type RetrievalEntry struct {
	Query       string    `json:"query"`
	ResultCount int       `json:"result_count"`
	TokensUsed  int       `json:"tokens_used"`
	WasUseful   bool      `json:"was_useful"`
	Timestamp   time.Time `json:"timestamp"`
	SessionID   string    `json:"session_id"`
	ToolContext string    `json:"tool_context,omitempty"`
}

// RetrievalReport summarizes retrieval effectiveness across sessions.
type RetrievalReport struct {
	TotalRecalls      int      `json:"total_recalls"`
	HitRate           float64  `json:"hit_rate"`
	AvgResultCount    float64  `json:"avg_result_count"`
	AvgTokensPerCall  int      `json:"avg_tokens_per_call"`
	TotalTokensSaved  int      `json:"total_tokens_saved"`
	MostQueriedTopics []string `json:"most_queried_topics"`
}

// NewRetrievalMetrics creates a metrics tracker that persists to disk.
func NewRetrievalMetrics(projectDir string) *RetrievalMetrics {
	savePath := ""
	if projectDir != "" {
		savePath = filepath.Join(projectDir, ".yaad", "retrieval_metrics.json")
	}
	rm := &RetrievalMetrics{
		entries:  make([]RetrievalEntry, 0, 256),
		savePath: savePath,
	}
	rm.load()
	return rm
}

// RecordRecall logs a memory recall operation.
func (rm *RetrievalMetrics) RecordRecall(query string, resultCount, tokensUsed int, toolCtx string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	entry := RetrievalEntry{
		Query:       query,
		ResultCount: resultCount,
		TokensUsed:  tokensUsed,
		WasUseful:   resultCount > 0,
		Timestamp:   time.Now(),
		ToolContext: toolCtx,
	}
	rm.entries = append(rm.entries, entry)

	if resultCount > 0 {
		rm.hitCount++
	} else {
		rm.missCount++
	}

	// Auto-save every 20 entries
	if len(rm.entries)%20 == 0 {
		rm.saveNoLock()
	}
}

// MarkUseful marks the most recent recall as useful (agent actually used the result).
func (rm *RetrievalMetrics) MarkUseful() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if len(rm.entries) > 0 {
		rm.entries[len(rm.entries)-1].WasUseful = true
	}
}

// HitRate returns the percentage of recalls that returned results.
func (rm *RetrievalMetrics) HitRate() float64 {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	total := rm.hitCount + rm.missCount
	if total == 0 {
		return 0
	}
	return float64(rm.hitCount) / float64(total)
}

// TotalRecalls returns how many recall operations have been performed.
func (rm *RetrievalMetrics) TotalRecalls() int {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return len(rm.entries)
}

// TokensSaved estimates tokens saved by using memory vs re-explaining.
// Assumes an average re-explanation costs ~500 tokens.
func (rm *RetrievalMetrics) TokensSaved() int {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	const avgReExplainCost = 500
	saved := 0
	for _, e := range rm.entries {
		if e.WasUseful {
			saved += avgReExplainCost - e.TokensUsed
		}
	}
	if saved < 0 {
		saved = 0
	}
	return saved
}

// Report generates a summary of retrieval effectiveness.
func (rm *RetrievalMetrics) Report() RetrievalReport {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	report := RetrievalReport{
		TotalRecalls: len(rm.entries),
	}

	if len(rm.entries) == 0 {
		return report
	}

	total := rm.hitCount + rm.missCount
	if total > 0 {
		report.HitRate = float64(rm.hitCount) / float64(total)
	}

	var totalResults int
	var totalTokens int
	topicCount := make(map[string]int)

	for _, e := range rm.entries {
		totalResults += e.ResultCount
		totalTokens += e.TokensUsed
		if e.Query != "" {
			topicCount[e.Query]++
		}
	}

	report.AvgResultCount = float64(totalResults) / float64(len(rm.entries))
	report.AvgTokensPerCall = totalTokens / len(rm.entries)
	report.TotalTokensSaved = rm.tokensSavedNoLock()

	// Top 5 queried topics
	type kv struct {
		Key   string
		Count int
	}
	var sorted []kv
	for k, v := range topicCount {
		sorted = append(sorted, kv{k, v})
	}
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Count > sorted[i].Count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	for i, kv := range sorted {
		if i >= 5 {
			break
		}
		report.MostQueriedTopics = append(report.MostQueriedTopics, kv.Key)
	}

	return report
}

func (rm *RetrievalMetrics) tokensSavedNoLock() int {
	const avgReExplainCost = 500
	saved := 0
	for _, e := range rm.entries {
		if e.WasUseful {
			saved += avgReExplainCost - e.TokensUsed
		}
	}
	if saved < 0 {
		saved = 0
	}
	return saved
}

// FormatSummary returns a human-readable summary string.
func (rm *RetrievalMetrics) FormatSummary() string {
	r := rm.Report()
	if r.TotalRecalls == 0 {
		return ""
	}
	return fmt.Sprintf("Memory: %d recalls (%.0f%% hit rate, ~%d tokens saved)",
		r.TotalRecalls, r.HitRate*100, r.TotalTokensSaved)
}

// Save persists metrics to disk.
func (rm *RetrievalMetrics) Save() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.saveNoLock()
}

func (rm *RetrievalMetrics) saveNoLock() {
	if rm.savePath == "" {
		return
	}
	dir := filepath.Dir(rm.savePath)
	_ = os.MkdirAll(dir, 0o755)

	// Keep only last 1000 entries
	if len(rm.entries) > 1000 {
		rm.entries = rm.entries[len(rm.entries)-1000:]
	}

	data, err := json.Marshal(rm.entries)
	if err != nil {
		return
	}
	_ = os.WriteFile(rm.savePath, data, 0o644)
}

func (rm *RetrievalMetrics) load() {
	if rm.savePath == "" {
		return
	}
	data, err := os.ReadFile(rm.savePath)
	if err != nil {
		return
	}
	var entries []RetrievalEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}
	rm.entries = entries
	for _, e := range entries {
		if e.WasUseful {
			rm.hitCount++
		} else {
			rm.missCount++
		}
	}
}
