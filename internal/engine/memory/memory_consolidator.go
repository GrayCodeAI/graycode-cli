package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/mathutil"
)

// RawMemory represents an unprocessed memory ingested from a session.
type RawMemory struct {
	Content   string    `json:"content"`
	Source    string    `json:"source"` // "session", "tool_output", "user_feedback"
	SessionID string    `json:"session_id"`
	Timestamp time.Time `json:"timestamp"`
	Processed bool      `json:"processed"`
}

// ConsolidatedMemory represents a processed, structured long-term memory.
type ConsolidatedMemory struct {
	ID         string     `json:"id"`
	Category   string     `json:"category"` // "fact", "convention", "decision", "warning", "skill"
	Content    string     `json:"content"`
	Confidence float64    `json:"confidence"`
	References []string   `json:"references"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// ConsolidatorStats holds aggregate statistics about the memory consolidator.
type ConsolidatorStats struct {
	RawCount          int       `json:"raw_count"`
	ProcessedCount    int       `json:"processed_count"`
	ConsolidatedCount int       `json:"consolidated_count"`
	FactCount         int       `json:"fact_count"`
	ConventionCount   int       `json:"convention_count"`
	DecisionCount     int       `json:"decision_count"`
	WarningCount      int       `json:"warning_count"`
	SkillCount        int       `json:"skill_count"`
	AvgConfidence     float64   `json:"avg_confidence"`
	LastConsolidation time.Time `json:"last_consolidation"`
}

// MemoryConsolidator processes raw session data into structured long-term memory
// during idle time, inspired by the "sleeptime" pattern.
type MemoryConsolidator struct {
	RawMemories          []RawMemory          `json:"raw_memories"`
	ConsolidatedMemories []ConsolidatedMemory `json:"consolidated_memories"`
	Dir                  string               `json:"-"`
	LastConsolidation    time.Time            `json:"last_consolidation"`
	mu                   sync.RWMutex
}

// NewMemoryConsolidator creates a new MemoryConsolidator that persists data in dir.
func NewMemoryConsolidator(dir string) *MemoryConsolidator {
	return &MemoryConsolidator{
		RawMemories:          []RawMemory{},
		ConsolidatedMemories: []ConsolidatedMemory{},
		Dir:                  dir,
	}
}

// Ingest adds raw content to the memory queue for later consolidation.
func (mc *MemoryConsolidator) Ingest(content, source, sessionID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.RawMemories = append(mc.RawMemories, RawMemory{
		Content:   content,
		Source:    source,
		SessionID: sessionID,
		Timestamp: time.Now(),
		Processed: false,
	})
}

// Consolidate processes unprocessed raw memories into structured consolidated memories.
// It extracts facts, conventions, decisions, and warnings, deduplicates them against
// existing consolidated memories, and marks raw memories as processed.
func (mc *MemoryConsolidator) Consolidate() ([]ConsolidatedMemory, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Gather unprocessed raw memories
	var unprocessed []RawMemory
	for i := range mc.RawMemories {
		if !mc.RawMemories[i].Processed {
			unprocessed = append(unprocessed, mc.RawMemories[i])
		}
	}

	if len(unprocessed) == 0 {
		return nil, nil
	}

	var newMemories []ConsolidatedMemory

	// Extract different categories
	facts := mc.ExtractFacts(unprocessed)
	conventions := mc.ExtractConventions(unprocessed)
	decisions := mc.ExtractDecisions(unprocessed)
	warnings := extractWarnings(unprocessed)

	newMemories = append(newMemories, facts...)
	newMemories = append(newMemories, conventions...)
	newMemories = append(newMemories, decisions...)
	newMemories = append(newMemories, warnings...)

	// Deduplicate against existing consolidated memories
	deduplicated := mc.deduplicate(newMemories)

	// Add deduplicated memories to the consolidated set
	mc.ConsolidatedMemories = append(mc.ConsolidatedMemories, deduplicated...)

	// Mark raw memories as processed
	for i := range mc.RawMemories {
		if !mc.RawMemories[i].Processed {
			mc.RawMemories[i].Processed = true
		}
	}

	mc.LastConsolidation = time.Now()

	return deduplicated, nil
}

// ExtractFacts identifies factual statements from raw memories.
// Patterns: "X is Y", "X uses Y", "X has Y"
func (mc *MemoryConsolidator) ExtractFacts(raw []RawMemory) []ConsolidatedMemory {
	var results []ConsolidatedMemory
	factPatterns := []string{" is ", " uses ", " has ", " runs ", " requires ", " depends on "}

	for _, r := range raw {
		lower := strings.ToLower(r.Content)
		for _, pattern := range factPatterns {
			if strings.Contains(lower, pattern) {
				id := fmt.Sprintf("fact-%d", time.Now().UnixNano())
				results = append(results, ConsolidatedMemory{
					ID:         id,
					Category:   "fact",
					Content:    r.Content,
					Confidence: confidenceFromSource(r.Source),
					References: []string{r.SessionID},
					CreatedAt:  time.Now(),
				})
				break
			}
		}
	}
	return results
}

// ExtractConventions identifies convention statements from raw memories.
// Patterns: "always", "never", "must", "should"
func (mc *MemoryConsolidator) ExtractConventions(raw []RawMemory) []ConsolidatedMemory {
	var results []ConsolidatedMemory
	conventionPatterns := []string{"always ", "never ", "must ", "should ", "convention", "rule is"}

	for _, r := range raw {
		lower := strings.ToLower(r.Content)
		for _, pattern := range conventionPatterns {
			if strings.Contains(lower, pattern) {
				id := fmt.Sprintf("conv-%d", time.Now().UnixNano())
				results = append(results, ConsolidatedMemory{
					ID:         id,
					Category:   "convention",
					Content:    r.Content,
					Confidence: confidenceFromSource(r.Source),
					References: []string{r.SessionID},
					CreatedAt:  time.Now(),
				})
				break
			}
		}
	}
	return results
}

// ExtractDecisions identifies decision statements from raw memories.
// Patterns: "decided", "chose", "went with", "because"
func (mc *MemoryConsolidator) ExtractDecisions(raw []RawMemory) []ConsolidatedMemory {
	var results []ConsolidatedMemory
	decisionPatterns := []string{"decided ", "chose ", "went with ", "because ", "decision to ", "opted for "}

	for _, r := range raw {
		lower := strings.ToLower(r.Content)
		for _, pattern := range decisionPatterns {
			if strings.Contains(lower, pattern) {
				id := fmt.Sprintf("dec-%d", time.Now().UnixNano())
				results = append(results, ConsolidatedMemory{
					ID:         id,
					Category:   "decision",
					Content:    r.Content,
					Confidence: confidenceFromSource(r.Source),
					References: []string{r.SessionID},
					CreatedAt:  time.Now(),
				})
				break
			}
		}
	}
	return results
}

// extractWarnings identifies warning statements from raw memories.
// Patterns: "don't", "avoid", "careful", "warning", "causes"
func extractWarnings(raw []RawMemory) []ConsolidatedMemory {
	var results []ConsolidatedMemory
	warningPatterns := []string{"don't ", "dont ", "avoid ", "careful ", "warning", " causes ", "breaks ", "dangerous"}

	for _, r := range raw {
		lower := strings.ToLower(r.Content)
		for _, pattern := range warningPatterns {
			if strings.Contains(lower, pattern) {
				id := fmt.Sprintf("warn-%d", time.Now().UnixNano())
				results = append(results, ConsolidatedMemory{
					ID:         id,
					Category:   "warning",
					Content:    r.Content,
					Confidence: confidenceFromSource(r.Source),
					References: []string{r.SessionID},
					CreatedAt:  time.Now(),
				})
				break
			}
		}
	}
	return results
}

// Recall searches consolidated memories by keyword relevance and returns up to limit results.
func (mc *MemoryConsolidator) Recall(query string, limit int) []ConsolidatedMemory {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	type scored struct {
		mem   ConsolidatedMemory
		score float64
	}

	queryWords := strings.Fields(strings.ToLower(query))
	var matches []scored

	now := time.Now()
	for _, mem := range mc.ConsolidatedMemories {
		// Skip expired memories
		if mem.ExpiresAt != nil && now.After(*mem.ExpiresAt) {
			continue
		}

		contentLower := strings.ToLower(mem.Content)
		var score float64

		for _, word := range queryWords {
			if strings.Contains(contentLower, word) {
				score += 1.0
			}
		}

		// Boost by confidence
		score *= mem.Confidence

		if score > 0 {
			matches = append(matches, scored{mem: mem, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	if len(matches) > limit {
		matches = matches[:limit]
	}

	results := make([]ConsolidatedMemory, len(matches))
	for i, m := range matches {
		results[i] = m.mem
	}
	return results
}

// Expire removes memories that have passed their expiration time.
func (mc *MemoryConsolidator) Expire() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	var active []ConsolidatedMemory
	for _, mem := range mc.ConsolidatedMemories {
		if mem.ExpiresAt == nil || now.Before(*mem.ExpiresAt) {
			active = append(active, mem)
		}
	}
	mc.ConsolidatedMemories = active
}

// Save persists the consolidator state to disk.
func (mc *MemoryConsolidator) Save() error {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if mc.Dir == "" {
		return fmt.Errorf("no directory configured for memory consolidator")
	}

	if err := os.MkdirAll(mc.Dir, 0o750); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}

	data, err := json.MarshalIndent(struct {
		RawMemories          []RawMemory          `json:"raw_memories"`
		ConsolidatedMemories []ConsolidatedMemory `json:"consolidated_memories"`
		LastConsolidation    time.Time            `json:"last_consolidation"`
	}{
		RawMemories:          mc.RawMemories,
		ConsolidatedMemories: mc.ConsolidatedMemories,
		LastConsolidation:    mc.LastConsolidation,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling memory consolidator: %w", err)
	}

	path := filepath.Join(mc.Dir, "consolidated_memory.json")
	return os.WriteFile(path, data, 0o600)
}

// Load restores the consolidator state from disk.
func (mc *MemoryConsolidator) Load() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.Dir == "" {
		return fmt.Errorf("no directory configured for memory consolidator")
	}

	path := filepath.Join(mc.Dir, "consolidated_memory.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No saved state yet
		}
		return fmt.Errorf("reading memory consolidator: %w", err)
	}

	var state struct {
		RawMemories          []RawMemory          `json:"raw_memories"`
		ConsolidatedMemories []ConsolidatedMemory `json:"consolidated_memories"`
		LastConsolidation    time.Time            `json:"last_consolidation"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unmarshaling memory consolidator: %w", err)
	}

	mc.RawMemories = state.RawMemories
	mc.ConsolidatedMemories = state.ConsolidatedMemories
	mc.LastConsolidation = state.LastConsolidation

	if mc.RawMemories == nil {
		mc.RawMemories = []RawMemory{}
	}
	if mc.ConsolidatedMemories == nil {
		mc.ConsolidatedMemories = []ConsolidatedMemory{}
	}

	return nil
}

// FormatMemories formats consolidated memories into a human-readable string.
func (mc *MemoryConsolidator) FormatMemories(memories []ConsolidatedMemory) string {
	if len(memories) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Consolidated Memories\n\n")

	// Group by category
	categories := map[string][]ConsolidatedMemory{}
	for _, m := range memories {
		categories[m.Category] = append(categories[m.Category], m)
	}

	categoryOrder := []string{"fact", "convention", "decision", "warning", "skill"}
	categoryLabels := map[string]string{
		"fact":       "Facts",
		"convention": "Conventions",
		"decision":   "Decisions",
		"warning":    "Warnings",
		"skill":      "Skills",
	}

	for _, cat := range categoryOrder {
		mems, ok := categories[cat]
		if !ok || len(mems) == 0 {
			continue
		}

		label := categoryLabels[cat]
		sb.WriteString(fmt.Sprintf("### %s\n", label))
		for _, m := range mems {
			sb.WriteString(fmt.Sprintf("- [%.0f%%] %s\n", m.Confidence*100, m.Content))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Stats returns aggregate statistics about the consolidator state.
func (mc *MemoryConsolidator) Stats() ConsolidatorStats {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	stats := ConsolidatorStats{
		RawCount:          len(mc.RawMemories),
		ConsolidatedCount: len(mc.ConsolidatedMemories),
		LastConsolidation: mc.LastConsolidation,
	}

	var totalConfidence float64
	for _, r := range mc.RawMemories {
		if r.Processed {
			stats.ProcessedCount++
		}
	}

	for _, m := range mc.ConsolidatedMemories {
		totalConfidence += m.Confidence
		switch m.Category {
		case "fact":
			stats.FactCount++
		case "convention":
			stats.ConventionCount++
		case "decision":
			stats.DecisionCount++
		case "warning":
			stats.WarningCount++
		case "skill":
			stats.SkillCount++
		}
	}

	if len(mc.ConsolidatedMemories) > 0 {
		stats.AvgConfidence = totalConfidence / float64(len(mc.ConsolidatedMemories))
	}

	return stats
}

// deduplicate removes memories that are too similar to existing consolidated memories.
func (mc *MemoryConsolidator) deduplicate(newMemories []ConsolidatedMemory) []ConsolidatedMemory {
	var unique []ConsolidatedMemory
	for _, newMem := range newMemories {
		isDuplicate := false
		for i, existing := range mc.ConsolidatedMemories {
			if existing.Category == newMem.Category && memorySimilar(existing.Content, newMem.Content) {
				// Boost confidence of existing memory instead of adding a duplicate
				mc.ConsolidatedMemories[i].Confidence = clampConfidenceVal(existing.Confidence + 0.1)
				// Merge references
				for _, ref := range newMem.References {
					if !memSliceContains(mc.ConsolidatedMemories[i].References, ref) {
						mc.ConsolidatedMemories[i].References = append(mc.ConsolidatedMemories[i].References, ref)
					}
				}
				isDuplicate = true
				break
			}
		}
		// Also check within the new batch
		if !isDuplicate {
			for _, already := range unique {
				if already.Category == newMem.Category && memorySimilar(already.Content, newMem.Content) {
					isDuplicate = true
					break
				}
			}
		}
		if !isDuplicate {
			unique = append(unique, newMem)
		}
	}
	return unique
}

// confidenceFromSource returns a base confidence score based on the memory source.
func confidenceFromSource(source string) float64 {
	switch source {
	case "user_feedback":
		return 0.9
	case "session":
		return 0.7
	case "tool_output":
		return 0.6
	default:
		return 0.5
	}
}

// memorySimilar checks if two memory contents are substantially similar.
func memorySimilar(a, b string) bool {
	aLower := strings.ToLower(a)
	bLower := strings.ToLower(b)

	// Exact match
	if aLower == bLower {
		return true
	}

	// One contains the other
	if strings.Contains(aLower, bLower) || strings.Contains(bLower, aLower) {
		return true
	}

	// Word overlap check
	aWords := strings.Fields(aLower)
	bWords := strings.Fields(bLower)

	if len(aWords) == 0 || len(bWords) == 0 {
		return false
	}

	overlap := 0
	for _, aw := range aWords {
		for _, bw := range bWords {
			if aw == bw {
				overlap++
				break
			}
		}
	}

	// If more than 70% of words overlap, consider similar
	minLen := len(aWords)
	if len(bWords) < minLen {
		minLen = len(bWords)
	}

	return float64(overlap)/float64(minLen) > 0.7
}

// clampConfidenceVal clamps a confidence value between 0.0 and 1.0.
func clampConfidenceVal(c float64) float64 {
	return mathutil.Clamp(c, 0, 1.0)
}

// memSliceContains checks if a string slice contains a specific string.
func memSliceContains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
