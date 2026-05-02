package engine

import (
	"fmt"
	"strings"
	"sync"
)

// Belief represents a single piece of discovered knowledge about the codebase.
type Belief struct {
	ID           string
	Category     string  // "file_purpose", "function_behavior", "dependency", "architecture"
	Subject      string  // file or symbol name
	Content      string  // what we believe about it
	Confidence   float64 // 0-1
	DiscoveredAt int     // turn index when discovered
	LastVerified int     // turn index when last confirmed
}

// BeliefState tracks what the agent has discovered about the codebase to prevent
// forgetting across long conversations. Beliefs are keyed by a generated ID.
type BeliefState struct {
	mu      sync.RWMutex
	beliefs map[string]*Belief
	nextID  int
}

// NewBeliefState creates an empty belief state.
func NewBeliefState() *BeliefState {
	return &BeliefState{
		beliefs: make(map[string]*Belief),
	}
}

// Record adds or updates a belief. If a belief with the same category and
// subject already exists, it is updated with the new content and turn index.
func (bs *BeliefState) Record(category, subject, content string, turn int) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Check for existing belief with same category + subject
	for _, b := range bs.beliefs {
		if b.Category == category && b.Subject == subject {
			b.Content = content
			b.Confidence = 1.0
			b.LastVerified = turn
			return
		}
	}

	// Create new belief
	bs.nextID++
	id := fmt.Sprintf("belief_%d", bs.nextID)
	bs.beliefs[id] = &Belief{
		ID:           id,
		Category:     category,
		Subject:      subject,
		Content:      content,
		Confidence:   1.0,
		DiscoveredAt: turn,
		LastVerified: turn,
	}
}

// Get returns all beliefs about a given subject.
func (bs *BeliefState) Get(subject string) []*Belief {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	var result []*Belief
	for _, b := range bs.beliefs {
		if b.Subject == subject {
			result = append(result, b)
		}
	}
	return result
}

// FormatForPrompt returns a formatted string summarizing all current beliefs,
// suitable for injection into the system prompt.
func (bs *BeliefState) FormatForPrompt() string {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	if len(bs.beliefs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("What you know so far:\n")

	// Group beliefs by category for readability
	categories := map[string][]*Belief{}
	for _, belief := range bs.beliefs {
		categories[belief.Category] = append(categories[belief.Category], belief)
	}

	for category, beliefs := range categories {
		b.WriteString(fmt.Sprintf("\n[%s]\n", category))
		for _, belief := range beliefs {
			b.WriteString(fmt.Sprintf("  - %s: %s", belief.Subject, belief.Content))
			if belief.Confidence < 1.0 {
				b.WriteString(fmt.Sprintf(" (confidence: %.0f%%)", belief.Confidence*100))
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// Invalidate marks all beliefs about a subject as stale by halving their
// confidence. This should be called when a file is modified, since our
// beliefs about it may no longer hold.
func (bs *BeliefState) Invalidate(subject string) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	for _, b := range bs.beliefs {
		if b.Subject == subject {
			b.Confidence *= 0.5
		}
	}
}

// Prune removes beliefs that have not been verified in the last 20 turns,
// keeping the belief state manageable in long conversations.
func (bs *BeliefState) Prune(currentTurn int) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	const staleTurnThreshold = 20

	for id, b := range bs.beliefs {
		if currentTurn-b.LastVerified > staleTurnThreshold {
			delete(bs.beliefs, id)
		}
	}
}

// Size returns the number of active beliefs.
func (bs *BeliefState) Size() int {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return len(bs.beliefs)
}
