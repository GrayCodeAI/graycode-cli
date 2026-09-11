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

	"github.com/GrayCodeAI/hawk/internal/safewrite"
)

// KnowledgeEntry represents a single piece of distilled knowledge.
type KnowledgeEntry struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Content    string    `json:"content"`
	Category   string    `json:"category"` // "pattern", "anti-pattern", "convention", "shortcut", "gotcha"
	Language   string    `json:"language"`
	Tags       []string  `json:"tags"`
	Examples   []string  `json:"examples"`
	Confidence float64   `json:"confidence"`
	UsageCount int       `json:"usage_count"`
	LastUsed   time.Time `json:"last_used"`
	CreatedAt  time.Time `json:"created_at"`
	Source     string    `json:"source"`
}

// KnowledgeStats holds aggregate statistics about the knowledge base.
type KnowledgeStats struct {
	TotalEntries  int            `json:"total_entries"`
	ByCategory    map[string]int `json:"by_category"`
	ByLanguage    map[string]int `json:"by_language"`
	AvgConfidence float64        `json:"avg_confidence"`
	MostUsed      []string       `json:"most_used"`
}

// KnowledgeBase stores distilled patterns extracted from successful sessions.
type KnowledgeBase struct {
	Entries    map[string]*KnowledgeEntry `json:"entries"`
	Categories map[string][]string        `json:"categories"`
	Dir        string                     `json:"-"`
	mu         sync.RWMutex
}

// NewKnowledgeBase creates a new KnowledgeBase that persists to the given directory.
func NewKnowledgeBase(dir string) *KnowledgeBase {
	return &KnowledgeBase{
		Entries:    make(map[string]*KnowledgeEntry),
		Categories: make(map[string][]string),
		Dir:        dir,
	}
}

// Add inserts a new knowledge entry into the base.
func (kb *KnowledgeBase) Add(entry *KnowledgeEntry) error {
	if entry == nil {
		return fmt.Errorf("knowledge: entry is nil")
	}
	if entry.ID == "" {
		return fmt.Errorf("knowledge: entry ID is required")
	}
	if entry.Category == "" {
		return fmt.Errorf("knowledge: entry category is required")
	}

	validCategories := map[string]bool{
		"pattern":      true,
		"anti-pattern": true,
		"convention":   true,
		"shortcut":     true,
		"gotcha":       true,
	}
	if !validCategories[entry.Category] {
		return fmt.Errorf("knowledge: invalid category %q", entry.Category)
	}

	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	kb.Entries[entry.ID] = entry

	// Update category index
	ids := kb.Categories[entry.Category]
	found := false
	for _, id := range ids {
		if id == entry.ID {
			found = true
			break
		}
	}
	if !found {
		kb.Categories[entry.Category] = append(kb.Categories[entry.Category], entry.ID)
	}

	return nil
}

// Search performs keyword search ranked by relevance, usage, and recency.
func (kb *KnowledgeBase) Search(query string, limit int) []*KnowledgeEntry {
	if query == "" || limit <= 0 {
		return nil
	}

	kb.mu.RLock()
	defer kb.mu.RUnlock()

	queryLower := strings.ToLower(query)
	words := strings.Fields(queryLower)

	type scored struct {
		entry *KnowledgeEntry
		score float64
	}

	var results []scored

	for _, entry := range kb.Entries {
		score := kb.scoreEntry(entry, queryLower, words)
		if score > 0 {
			results = append(results, scored{entry: entry, score: score})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	entries := make([]*KnowledgeEntry, len(results))
	for i, r := range results {
		entries[i] = r.entry
	}
	return entries
}

func (kb *KnowledgeBase) scoreEntry(entry *KnowledgeEntry, queryLower string, words []string) float64 {
	var score float64

	titleLower := strings.ToLower(entry.Title)
	contentLower := strings.ToLower(entry.Content)
	tagsLower := strings.ToLower(strings.Join(entry.Tags, " "))

	// Exact phrase match in title is highest value
	if strings.Contains(titleLower, queryLower) {
		score += 10.0
	}

	// Exact phrase match in content
	if strings.Contains(contentLower, queryLower) {
		score += 5.0
	}

	// Individual word matches
	for _, word := range words {
		if strings.Contains(titleLower, word) {
			score += 3.0
		}
		if strings.Contains(contentLower, word) {
			score += 1.0
		}
		if strings.Contains(tagsLower, word) {
			score += 2.0
		}
		if strings.EqualFold(entry.Language, word) {
			score += 2.0
		}
		if strings.EqualFold(entry.Category, word) {
			score += 1.5
		}
	}

	if score == 0 {
		return 0
	}

	// Boost by usage count (log scale)
	if entry.UsageCount > 0 {
		score += float64(entry.UsageCount) * 0.5
	}

	// Boost by recency (entries used in the last 24h get a bonus)
	if !entry.LastUsed.IsZero() {
		hoursSinceUse := time.Since(entry.LastUsed).Hours()
		if hoursSinceUse < 24 {
			score += 2.0
		} else if hoursSinceUse < 168 { // 1 week
			score += 1.0
		}
	}

	// Boost by confidence
	score *= entry.Confidence

	return score
}

// GetByCategory returns all entries in the given category.
func (kb *KnowledgeBase) GetByCategory(category string) []*KnowledgeEntry {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	ids, ok := kb.Categories[category]
	if !ok {
		return nil
	}

	var entries []*KnowledgeEntry
	for _, id := range ids {
		if entry, exists := kb.Entries[id]; exists {
			entries = append(entries, entry)
		}
	}
	return entries
}

// ExtractFromSession uses heuristics to extract knowledge entries from a session's messages and outcome.
func (kb *KnowledgeBase) ExtractFromSession(messages []string, outcome string) []*KnowledgeEntry {
	if len(messages) == 0 {
		return nil
	}

	var extracted []*KnowledgeEntry

	// Heuristic 1: Look for corrections (messages containing "actually", "instead", "wrong", "should be")
	correctionMarkers := []string{"actually", "instead", "wrong", "should be", "correction", "mistake", "not correct"}
	for i, msg := range messages {
		msgLower := strings.ToLower(msg)
		for _, marker := range correctionMarkers {
			if strings.Contains(msgLower, marker) {
				entry := &KnowledgeEntry{
					ID:         fmt.Sprintf("session-correction-%d", i),
					Title:      extractTitle(msg, "Correction"),
					Content:    msg,
					Category:   "gotcha",
					Tags:       []string{"correction", "learned"},
					Confidence: 0.6,
					CreatedAt:  time.Now(),
					Source:     "session-extraction",
				}
				extracted = append(extracted, entry)
				break
			}
		}
	}

	// Heuristic 2: Look for discoveries (messages containing "found", "discovered", "turns out", "TIL")
	discoveryMarkers := []string{"found that", "discovered", "turns out", "til ", "learned that", "realized"}
	for i, msg := range messages {
		msgLower := strings.ToLower(msg)
		for _, marker := range discoveryMarkers {
			if strings.Contains(msgLower, marker) {
				entry := &KnowledgeEntry{
					ID:         fmt.Sprintf("session-discovery-%d", i),
					Title:      extractTitle(msg, "Discovery"),
					Content:    msg,
					Category:   "pattern",
					Tags:       []string{"discovery", "learned"},
					Confidence: 0.5,
					CreatedAt:  time.Now(),
					Source:     "session-extraction",
				}
				extracted = append(extracted, entry)
				break
			}
		}
	}

	// Heuristic 3: Look for conventions (messages containing "always", "never", "rule", "convention")
	conventionMarkers := []string{"always ", "never ", "rule:", "convention:", "standard:", "best practice"}
	for i, msg := range messages {
		msgLower := strings.ToLower(msg)
		for _, marker := range conventionMarkers {
			if strings.Contains(msgLower, marker) {
				entry := &KnowledgeEntry{
					ID:         fmt.Sprintf("session-convention-%d", i),
					Title:      extractTitle(msg, "Convention"),
					Content:    msg,
					Category:   "convention",
					Tags:       []string{"convention", "learned"},
					Confidence: 0.7,
					CreatedAt:  time.Now(),
					Source:     "session-extraction",
				}
				extracted = append(extracted, entry)
				break
			}
		}
	}

	// Heuristic 4: Look for shortcuts (messages containing "shortcut", "quick", "faster", "easier")
	shortcutMarkers := []string{"shortcut", "quick way", "faster", "easier way", "trick:", "tip:"}
	for i, msg := range messages {
		msgLower := strings.ToLower(msg)
		for _, marker := range shortcutMarkers {
			if strings.Contains(msgLower, marker) {
				entry := &KnowledgeEntry{
					ID:         fmt.Sprintf("session-shortcut-%d", i),
					Title:      extractTitle(msg, "Shortcut"),
					Content:    msg,
					Category:   "shortcut",
					Tags:       []string{"shortcut", "efficiency"},
					Confidence: 0.55,
					CreatedAt:  time.Now(),
					Source:     "session-extraction",
				}
				extracted = append(extracted, entry)
				break
			}
		}
	}

	// Boost confidence for entries from successful outcomes
	if strings.Contains(strings.ToLower(outcome), "success") || strings.Contains(strings.ToLower(outcome), "completed") {
		for _, entry := range extracted {
			entry.Confidence = minFloat(entry.Confidence+0.2, 1.0)
		}
	}

	return extracted
}

// BuildContextForTask formats relevant knowledge for prompt injection, respecting a token budget.
func (kb *KnowledgeBase) BuildContextForTask(task string, maxTokens int) string {
	if task == "" || maxTokens <= 0 {
		return ""
	}

	// Estimate ~4 chars per token
	maxChars := maxTokens * 4

	entries := kb.Search(task, 20)
	if len(entries) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("# Relevant Knowledge\n\n")
	remaining := maxChars - builder.Len()

	for _, entry := range entries {
		formatted := kb.FormatEntry(entry)
		if len(formatted)+2 > remaining {
			break
		}
		builder.WriteString(formatted)
		builder.WriteString("\n")
		remaining -= len(formatted) + 1

		// Mark as used
		kb.mu.Lock()
		entry.UsageCount++
		entry.LastUsed = time.Now()
		kb.mu.Unlock()
	}

	return builder.String()
}

// Merge combines another KnowledgeBase into this one, deduplicating by content similarity.
func (kb *KnowledgeBase) Merge(other *KnowledgeBase) {
	if other == nil {
		return
	}

	other.mu.RLock()
	defer other.mu.RUnlock()

	for _, entry := range other.Entries {
		if kb.isDuplicate(entry) {
			// Merge usage stats into existing
			kb.mu.Lock()
			for _, existing := range kb.Entries {
				if kb.isSimilar(existing, entry) {
					existing.UsageCount += entry.UsageCount
					if entry.LastUsed.After(existing.LastUsed) {
						existing.LastUsed = entry.LastUsed
					}
					if entry.Confidence > existing.Confidence {
						existing.Confidence = entry.Confidence
					}
					break
				}
			}
			kb.mu.Unlock()
			continue
		}
		_ = kb.Add(entry)
	}
}

func (kb *KnowledgeBase) isDuplicate(entry *KnowledgeEntry) bool {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	// Check exact ID match
	if _, exists := kb.Entries[entry.ID]; exists {
		return true
	}

	// Check content similarity
	for _, existing := range kb.Entries {
		if kb.isSimilar(existing, entry) {
			return true
		}
	}
	return false
}

func (kb *KnowledgeBase) isSimilar(a, b *KnowledgeEntry) bool {
	// Simple similarity: same title or high content overlap
	if a.Title != "" && a.Title == b.Title {
		return true
	}

	// Jaccard-like similarity on content words
	if a.Content == "" || b.Content == "" {
		return false
	}

	wordsA := strings.Fields(strings.ToLower(a.Content))
	wordsB := strings.Fields(strings.ToLower(b.Content))

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false
	}

	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
	}

	intersection := 0
	for _, w := range wordsB {
		if setA[w] {
			intersection++
		}
	}

	union := len(setA) + len(wordsB) - intersection
	if union == 0 {
		return false
	}

	similarity := float64(intersection) / float64(union)
	return similarity > 0.8
}

// Prune removes entries below minimum confidence or older than maxAge.
func (kb *KnowledgeBase) Prune(minConfidence float64, maxAge time.Duration) {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	now := time.Now()
	var toRemove []string

	for id, entry := range kb.Entries {
		if entry.Confidence < minConfidence {
			toRemove = append(toRemove, id)
			continue
		}
		if maxAge > 0 && now.Sub(entry.CreatedAt) > maxAge {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		entry := kb.Entries[id]
		if entry != nil {
			// Remove from category index
			cat := entry.Category
			ids := kb.Categories[cat]
			for i, eid := range ids {
				if eid == id {
					kb.Categories[cat] = append(ids[:i], ids[i+1:]...)
					break
				}
			}
		}
		delete(kb.Entries, id)
	}
}

// Save persists the knowledge base to JSON files in the configured directory.
func (kb *KnowledgeBase) Save() error {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	if kb.Dir == "" {
		return fmt.Errorf("knowledge: directory not configured")
	}

	if err := os.MkdirAll(kb.Dir, 0o750); err != nil {
		return fmt.Errorf("knowledge: create dir: %w", err)
	}

	data := struct {
		Entries    map[string]*KnowledgeEntry `json:"entries"`
		Categories map[string][]string        `json:"categories"`
	}{
		Entries:    kb.Entries,
		Categories: kb.Categories,
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("knowledge: marshal: %w", err)
	}

	path := filepath.Join(kb.Dir, "knowledge.json")
	if err := safewrite.WriteFile(path, raw); err != nil {
		return fmt.Errorf("knowledge: write: %w", err)
	}

	return nil
}

// Load reads the knowledge base from disk.
func (kb *KnowledgeBase) Load() error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	if kb.Dir == "" {
		return fmt.Errorf("knowledge: directory not configured")
	}

	path := filepath.Join(kb.Dir, "knowledge.json")
	raw, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No existing data is not an error
		}
		return fmt.Errorf("knowledge: read: %w", err)
	}

	var data struct {
		Entries    map[string]*KnowledgeEntry `json:"entries"`
		Categories map[string][]string        `json:"categories"`
	}

	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("knowledge: unmarshal: %w", err)
	}

	if data.Entries != nil {
		kb.Entries = data.Entries
	}
	if data.Categories != nil {
		kb.Categories = data.Categories
	}

	return nil
}

// Stats returns aggregate statistics about the knowledge base.
func (kb *KnowledgeBase) Stats() KnowledgeStats {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	stats := KnowledgeStats{
		TotalEntries: len(kb.Entries),
		ByCategory:   make(map[string]int),
		ByLanguage:   make(map[string]int),
	}

	var totalConfidence float64
	type usageEntry struct {
		id    string
		count int
	}
	var usageList []usageEntry

	for id, entry := range kb.Entries {
		stats.ByCategory[entry.Category]++
		if entry.Language != "" {
			stats.ByLanguage[entry.Language]++
		}
		totalConfidence += entry.Confidence
		usageList = append(usageList, usageEntry{id: id, count: entry.UsageCount})
	}

	if stats.TotalEntries > 0 {
		stats.AvgConfidence = totalConfidence / float64(stats.TotalEntries)
	}

	sort.Slice(usageList, func(i, j int) bool {
		return usageList[i].count > usageList[j].count
	})

	topN := 5
	if len(usageList) < topN {
		topN = len(usageList)
	}
	for i := 0; i < topN; i++ {
		if usageList[i].count > 0 {
			stats.MostUsed = append(stats.MostUsed, usageList[i].id)
		}
	}

	return stats
}

// FormatEntry formats a knowledge entry for display or prompt injection.
func (kb *KnowledgeBase) FormatEntry(entry *KnowledgeEntry) string {
	if entry == nil {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("## %s [%s]\n", entry.Title, entry.Category))

	if entry.Language != "" {
		builder.WriteString(fmt.Sprintf("Language: %s\n", entry.Language))
	}

	builder.WriteString(entry.Content)
	builder.WriteString("\n")

	if len(entry.Examples) > 0 {
		builder.WriteString("Examples:\n")
		for _, ex := range entry.Examples {
			builder.WriteString(fmt.Sprintf("  - %s\n", ex))
		}
	}

	if len(entry.Tags) > 0 {
		builder.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(entry.Tags, ", ")))
	}

	return builder.String()
}

// extractTitle creates a short title from the first sentence or N words of a message.
func extractTitle(msg, fallback string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return fallback
	}

	// Take first sentence
	for _, sep := range []string{". ", "! ", "? ", "\n"} {
		if idx := strings.Index(msg, sep); idx > 0 && idx < 80 {
			return msg[:idx]
		}
	}

	// Truncate to ~80 chars
	if len(msg) > 80 {
		return msg[:78] + "..."
	}
	return msg
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
