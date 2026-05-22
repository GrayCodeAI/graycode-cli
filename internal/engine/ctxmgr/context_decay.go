package ctxmgr

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ContextDecay manages context entries with time-based importance decay.
// Older context gradually loses weight unless pinned or accessed, keeping
// the most relevant information prioritized for the model's context window.
//
// The decay follows exponential half-life: weight = initial * 0.5^(elapsed/halfLife)
// This mirrors human memory curves and ensures stale context doesn't crowd out
// fresh, relevant information.
type ContextDecay struct {
	HalfLife  time.Duration
	MinWeight float64
	Entries   []DecayEntry
	mu        sync.RWMutex
}

// DecayEntry represents a single piece of context with decay metadata.
type DecayEntry struct {
	ID           string
	Content      string
	Weight       float64
	CreatedAt    time.Time
	LastAccessed time.Time
	AccessCount  int
	Category     string
	Tokens       int
	Pinned       bool
}

// DecayStats provides aggregate statistics about the context decay state.
type DecayStats struct {
	TotalEntries int
	AvgWeight    float64
	Oldest       time.Time
	Newest       time.Time
	PinnedCount  int
	TotalTokens  int
}

// NewContextDecay creates a new ContextDecay manager with the given half-life.
// If halfLife is zero or negative, a default of 30 minutes is used.
func NewContextDecay(halfLife time.Duration) *ContextDecay {
	if halfLife <= 0 {
		halfLife = 30 * time.Minute
	}
	return &ContextDecay{
		HalfLife:  halfLife,
		MinWeight: 0.1,
		Entries:   make([]DecayEntry, 0),
	}
}

// Add inserts a new context entry with initial weight 1.0.
// Returns the generated ID for the entry.
func (cd *ContextDecay) Add(content, category string, tokens int) string {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	now := time.Now()
	id := fmt.Sprintf("ctx_%d_%d", now.UnixNano(), len(cd.Entries))

	entry := DecayEntry{
		ID:           id,
		Content:      content,
		Weight:       1.0,
		CreatedAt:    now,
		LastAccessed: now,
		AccessCount:  0,
		Category:     category,
		Tokens:       tokens,
		Pinned:       false,
	}

	cd.Entries = append(cd.Entries, entry)
	return id
}

// Get retrieves an entry by ID and returns it with its current decayed weight.
// Returns nil if the entry is not found.
func (cd *ContextDecay) Get(id string) (*DecayEntry, float64) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	for i := range cd.Entries {
		if cd.Entries[i].ID == id {
			entry := &cd.Entries[i]
			weight := cd.calculateWeight(entry)
			return entry, weight
		}
	}
	return nil, 0
}

// calculateWeight computes the current decayed weight for an entry.
func (cd *ContextDecay) calculateWeight(entry *DecayEntry) float64 {
	if entry.Pinned {
		return 1.0
	}

	elapsed := time.Since(entry.LastAccessed)
	halfLives := float64(elapsed) / float64(cd.HalfLife)
	weight := entry.Weight * math.Pow(0.5, halfLives)

	if weight < cd.MinWeight {
		return cd.MinWeight
	}
	return weight
}

// ApplyDecay recalculates all entry weights based on time elapsed since last access.
// Pinned entries are not affected by decay.
func (cd *ContextDecay) ApplyDecay() {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	for i := range cd.Entries {
		if cd.Entries[i].Pinned {
			cd.Entries[i].Weight = 1.0
			continue
		}

		elapsed := time.Since(cd.Entries[i].LastAccessed)
		halfLives := float64(elapsed) / float64(cd.HalfLife)
		newWeight := math.Pow(0.5, halfLives)

		if newWeight < cd.MinWeight {
			newWeight = cd.MinWeight
		}
		cd.Entries[i].Weight = newWeight
	}
}

// Access marks an entry as accessed, boosting its weight as a relevance signal.
// Each access resets the decay timer and increases the base weight.
func (cd *ContextDecay) Access(id string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	for i := range cd.Entries {
		if cd.Entries[i].ID == id {
			cd.Entries[i].LastAccessed = time.Now()
			cd.Entries[i].AccessCount++
			// Boost weight back toward 1.0 on access
			cd.Entries[i].Weight = math.Min(1.0, cd.Entries[i].Weight+0.2)
			return
		}
	}
}

// Pin marks an entry as pinned, preventing decay. Pinned entries always have weight 1.0.
func (cd *ContextDecay) Pin(id string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	for i := range cd.Entries {
		if cd.Entries[i].ID == id {
			cd.Entries[i].Pinned = true
			cd.Entries[i].Weight = 1.0
			return
		}
	}
}

// Unpin removes the pin from an entry, allowing normal decay to resume.
func (cd *ContextDecay) Unpin(id string) {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	for i := range cd.Entries {
		if cd.Entries[i].ID == id {
			cd.Entries[i].Pinned = false
			return
		}
	}
}

// GetTopN returns the N entries with the highest current weight, sorted descending.
func (cd *ContextDecay) GetTopN(n int) []DecayEntry {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	// Create a copy with current weights
	weighted := make([]DecayEntry, len(cd.Entries))
	copy(weighted, cd.Entries)

	for i := range weighted {
		weighted[i].Weight = cd.calculateWeight(&cd.Entries[i])
	}

	sort.Slice(weighted, func(a, b int) bool {
		return weighted[a].Weight > weighted[b].Weight
	})

	if n > len(weighted) {
		n = len(weighted)
	}
	return weighted[:n]
}

// GetByBudget returns entries fitting within the given token budget, sorted by weight descending.
// Entries are selected greedily by weight until the budget is exhausted.
func (cd *ContextDecay) GetByBudget(maxTokens int) []DecayEntry {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	// Create a copy with current weights
	weighted := make([]DecayEntry, len(cd.Entries))
	copy(weighted, cd.Entries)

	for i := range weighted {
		weighted[i].Weight = cd.calculateWeight(&cd.Entries[i])
	}

	sort.Slice(weighted, func(a, b int) bool {
		return weighted[a].Weight > weighted[b].Weight
	})

	var result []DecayEntry
	usedTokens := 0

	for _, entry := range weighted {
		if usedTokens+entry.Tokens > maxTokens {
			continue
		}
		result = append(result, entry)
		usedTokens += entry.Tokens
	}

	return result
}

// Prune removes all entries whose current decayed weight is below the given threshold.
// Returns the number of entries removed.
func (cd *ContextDecay) Prune(minWeight float64) int {
	cd.mu.Lock()
	defer cd.mu.Unlock()

	kept := make([]DecayEntry, 0, len(cd.Entries))
	removed := 0

	for i := range cd.Entries {
		w := cd.calculateWeight(&cd.Entries[i])
		if w >= minWeight || cd.Entries[i].Pinned {
			kept = append(kept, cd.Entries[i])
		} else {
			removed++
		}
	}

	cd.Entries = kept
	return removed
}

// BuildContext formats the top entries fitting within the token budget as a context string.
func (cd *ContextDecay) BuildContext(maxTokens int) string {
	entries := cd.GetByBudget(maxTokens)
	return cd.FormatEntries(entries)
}

// FormatEntries renders a slice of entries as a human-readable context block
// showing weight, pin status, and content.
func (cd *ContextDecay) FormatEntries(entries []DecayEntry) string {
	if len(entries) == 0 {
		return "Context (decayed):\n─────────────────\n(empty)\n"
	}

	var b strings.Builder
	b.WriteString("Context (decayed):\n")
	b.WriteString("─────────────────\n")

	for _, entry := range entries {
		pin := ""
		if entry.Pinned {
			pin = " \U0001f4cc"
		}

		fading := ""
		if entry.Weight < 0.2 {
			fading = " (fading)"
		}

		b.WriteString(fmt.Sprintf("[%.2f]%s %s%s\n", entry.Weight, pin, entry.Content, fading))
	}

	return b.String()
}

// Stats returns aggregate statistics about the current context decay state.
func (cd *ContextDecay) Stats() DecayStats {
	cd.mu.RLock()
	defer cd.mu.RUnlock()

	stats := DecayStats{}
	if len(cd.Entries) == 0 {
		return stats
	}

	stats.TotalEntries = len(cd.Entries)
	var totalWeight float64
	stats.Oldest = cd.Entries[0].CreatedAt
	stats.Newest = cd.Entries[0].CreatedAt

	for i := range cd.Entries {
		w := cd.calculateWeight(&cd.Entries[i])
		totalWeight += w
		stats.TotalTokens += cd.Entries[i].Tokens

		if cd.Entries[i].Pinned {
			stats.PinnedCount++
		}
		if cd.Entries[i].CreatedAt.Before(stats.Oldest) {
			stats.Oldest = cd.Entries[i].CreatedAt
		}
		if cd.Entries[i].CreatedAt.After(stats.Newest) {
			stats.Newest = cd.Entries[i].CreatedAt
		}
	}

	stats.AvgWeight = totalWeight / float64(stats.TotalEntries)
	return stats
}
