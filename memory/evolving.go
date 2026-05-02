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
)

// UsageGuideline represents a lesson learned from problem-solving experience.
type UsageGuideline struct {
	ID         string    `json:"id"`
	Pattern    string    `json:"pattern"`    // when this situation occurs
	Lesson     string    `json:"lesson"`     // what to do / not do
	Source     string    `json:"source"`     // which session taught this
	Confidence float64   `json:"confidence"` // 0-1, increases with repeated confirmation
	Uses       int       `json:"uses"`       // how many times retrieved
	CreatedAt  time.Time `json:"created_at"`
}

// EvolvingMemory manages usage guidelines learned from problem-solving.
type EvolvingMemory struct {
	mu         sync.Mutex
	guidelines []UsageGuideline
	path       string // ~/.hawk/memory/guidelines.json
}

// NewEvolvingMemory creates a new EvolvingMemory with the default storage path.
func NewEvolvingMemory() *EvolvingMemory {
	home, _ := os.UserHomeDir()
	return &EvolvingMemory{
		path: filepath.Join(home, ".hawk", "memory", "guidelines.json"),
	}
}

// Load reads persisted guidelines from disk.
func (em *EvolvingMemory) Load() error {
	em.mu.Lock()
	defer em.mu.Unlock()

	data, err := os.ReadFile(em.path)
	if err != nil {
		if os.IsNotExist(err) {
			em.guidelines = nil
			return nil
		}
		return fmt.Errorf("load guidelines: %w", err)
	}
	var guidelines []UsageGuideline
	if err := json.Unmarshal(data, &guidelines); err != nil {
		return fmt.Errorf("parse guidelines: %w", err)
	}
	em.guidelines = guidelines
	return nil
}

// Save persists guidelines to disk.
func (em *EvolvingMemory) Save() error {
	em.mu.Lock()
	defer em.mu.Unlock()

	dir := filepath.Dir(em.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	data, err := json.MarshalIndent(em.guidelines, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal guidelines: %w", err)
	}
	return os.WriteFile(em.path, data, 0o644)
}

// Learn adds a new guideline or strengthens an existing one if a similar pattern exists.
func (em *EvolvingMemory) Learn(pattern, lesson, source string) {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Check if a similar guideline already exists (by pattern keyword overlap).
	patternLower := strings.ToLower(pattern)
	for i, g := range em.guidelines {
		if keywordOverlap(strings.ToLower(g.Pattern), patternLower) > 0.5 {
			// Strengthen existing guideline.
			em.guidelines[i].Confidence = clampFloat(g.Confidence+0.1, 0, 1)
			em.guidelines[i].Uses++
			if lesson != "" && lesson != g.Lesson {
				em.guidelines[i].Lesson = g.Lesson + "; " + lesson
			}
			return
		}
	}

	// Add new guideline.
	em.guidelines = append(em.guidelines, UsageGuideline{
		ID:         fmt.Sprintf("guide_%d", time.Now().UnixNano()),
		Pattern:    pattern,
		Lesson:     lesson,
		Source:     source,
		Confidence: 0.5,
		Uses:       0,
		CreatedAt:  time.Now(),
	})
}

// Retrieve returns the top-K guidelines most relevant to the given context string.
func (em *EvolvingMemory) Retrieve(context string, topK int) []UsageGuideline {
	em.mu.Lock()
	defer em.mu.Unlock()

	if topK <= 0 {
		topK = 5
	}
	contextLower := strings.ToLower(context)
	contextTokens := tokenizeSimple(contextLower)

	type scored struct {
		idx   int
		score float64
	}
	var results []scored

	for i, g := range em.guidelines {
		patternTokens := tokenizeSimple(strings.ToLower(g.Pattern))
		overlap := tokenOverlap(contextTokens, patternTokens)
		if overlap > 0 {
			// Score = overlap * confidence
			score := overlap * g.Confidence
			results = append(results, scored{idx: i, score: score})
		}
	}

	sort.Slice(results, func(a, b int) bool {
		return results[a].score > results[b].score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	out := make([]UsageGuideline, len(results))
	for i, r := range results {
		out[i] = em.guidelines[r.idx]
	}
	return out
}

// Strengthen increases confidence for a guideline that was useful.
func (em *EvolvingMemory) Strengthen(id string) {
	em.mu.Lock()
	defer em.mu.Unlock()

	for i, g := range em.guidelines {
		if g.ID == id {
			em.guidelines[i].Confidence = clampFloat(g.Confidence+0.1, 0, 1)
			em.guidelines[i].Uses++
			return
		}
	}
}

// Decay reduces confidence of unused guidelines over time.
// Guidelines with confidence below 0.1 are removed.
func (em *EvolvingMemory) Decay() {
	em.mu.Lock()
	defer em.mu.Unlock()

	var kept []UsageGuideline
	for _, g := range em.guidelines {
		g.Confidence = clampFloat(g.Confidence-0.05, 0, 1)
		if g.Confidence >= 0.1 {
			kept = append(kept, g)
		}
	}
	em.guidelines = kept
}

// Format returns guidelines formatted for prompt injection.
func (em *EvolvingMemory) Format(topK int) string {
	em.mu.Lock()
	defer em.mu.Unlock()

	if len(em.guidelines) == 0 {
		return ""
	}
	if topK <= 0 {
		topK = 5
	}

	// Sort by confidence descending.
	sorted := make([]UsageGuideline, len(em.guidelines))
	copy(sorted, em.guidelines)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Confidence > sorted[j].Confidence
	})

	if len(sorted) > topK {
		sorted = sorted[:topK]
	}

	var b strings.Builder
	b.WriteString("## Usage Guidelines (learned from experience)\n\n")
	for _, g := range sorted {
		b.WriteString(fmt.Sprintf("- When: %s\n  Do: %s (confidence: %.0f%%)\n", g.Pattern, g.Lesson, g.Confidence*100))
	}
	return b.String()
}

// Guidelines returns a copy of all current guidelines.
func (em *EvolvingMemory) Guidelines() []UsageGuideline {
	em.mu.Lock()
	defer em.mu.Unlock()
	out := make([]UsageGuideline, len(em.guidelines))
	copy(out, em.guidelines)
	return out
}

// keywordOverlap computes the Jaccard similarity between two strings by words.
func keywordOverlap(a, b string) float64 {
	tokensA := tokenizeSimple(a)
	tokensB := tokenizeSimple(b)
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0
	}

	setA := make(map[string]bool, len(tokensA))
	for _, t := range tokensA {
		setA[t] = true
	}
	setB := make(map[string]bool, len(tokensB))
	for _, t := range tokensB {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}

	union := len(setA)
	for t := range setB {
		if !setA[t] {
			union++
		}
	}

	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// tokenOverlap computes the fraction of contextTokens that appear in patternTokens.
func tokenOverlap(contextTokens, patternTokens []string) float64 {
	if len(patternTokens) == 0 {
		return 0
	}
	patternSet := make(map[string]bool, len(patternTokens))
	for _, t := range patternTokens {
		patternSet[t] = true
	}
	matches := 0
	for _, t := range contextTokens {
		if patternSet[t] {
			matches++
		}
	}
	return float64(matches) / float64(len(patternTokens))
}

// tokenizeSimple splits text into lowercase words.
func tokenizeSimple(text string) []string {
	fields := strings.Fields(text)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, ".,;:!?()[]{}\"'")
		if len(f) > 1 {
			out = append(out, f)
		}
	}
	return out
}

// clampFloat clamps v to [lo, hi].
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
