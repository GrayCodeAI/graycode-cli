package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

// maxSelfImproveEntries caps the persisted lesson store so a long-lived
// machine does not accumulate an unbounded file. Oldest entries are dropped.
const maxSelfImproveEntries = 200

// SelfImproveEntry records a lesson learned from a mistake.
type SelfImproveEntry struct {
	Timestamp time.Time `json:"timestamp"`
	What      string    `json:"what"`     // what went wrong
	Why       string    `json:"why"`      // root cause
	Lesson    string    `json:"lesson"`   // what to do differently
	Category  string    `json:"category"` // code, test, design, communication
}

// SelfImprover tracks mistakes and lessons across sessions.
type SelfImprover struct {
	Path    string
	Entries []SelfImproveEntry
	mu      sync.Mutex
}

// NewSelfImprover loads or creates the improvement log.
func NewSelfImprover() *SelfImprover {
	path := filepath.Join(storage.StateDir(), "self-improve.json")
	si := &SelfImprover{Path: path}
	si.load()
	return si
}

// Learn records a new lesson. It is nil-safe and bounded: oldest entries are
// dropped past maxSelfImproveEntries so the store cannot grow without limit.
// Exact duplicates (same what/lesson/category) are skipped so repeated
// failures do not fill the store with identical rows.
func (si *SelfImprover) Learn(what, why, lesson, category string) {
	if si == nil {
		return
	}
	si.mu.Lock()
	defer si.mu.Unlock()
	for _, e := range si.Entries {
		if e.What == what && e.Lesson == lesson && e.Category == category {
			return
		}
	}
	si.Entries = append(si.Entries, SelfImproveEntry{
		Timestamp: time.Now(),
		What:      what,
		Why:       why,
		Lesson:    lesson,
		Category:  category,
	})
	if len(si.Entries) > maxSelfImproveEntries {
		si.Entries = si.Entries[len(si.Entries)-maxSelfImproveEntries:]
	}
	si.save()
}

// Lessons returns all lessons, optionally filtered by category.
func (si *SelfImprover) Lessons(category string) []SelfImproveEntry {
	if si == nil {
		return nil
	}
	si.mu.Lock()
	defer si.mu.Unlock()
	if category == "" {
		return append([]SelfImproveEntry{}, si.Entries...)
	}
	var filtered []SelfImproveEntry
	for _, e := range si.Entries {
		if e.Category == category {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// Clear removes all lessons.
func (si *SelfImprover) Clear() {
	if si == nil {
		return
	}
	si.mu.Lock()
	defer si.mu.Unlock()
	if len(si.Entries) == 0 {
		return
	}
	si.Entries = nil
	si.save()
}

// ForPrompt formats recent lessons as context for the system prompt.
func (si *SelfImprover) ForPrompt(maxEntries int) string {
	if si == nil {
		return ""
	}
	si.mu.Lock()
	defer si.mu.Unlock()
	if len(si.Entries) == 0 {
		return ""
	}
	start := len(si.Entries) - maxEntries
	if start < 0 {
		start = 0
	}
	result := "## Lessons Learned (avoid repeating these mistakes)\n"
	for _, e := range si.Entries[start:] {
		result += fmt.Sprintf("- [%s] %s → %s\n", e.Category, e.What, e.Lesson)
	}
	return result
}

func (si *SelfImprover) load() {
	data, err := os.ReadFile(si.Path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &si.Entries)
}

func (si *SelfImprover) save() {
	_ = os.MkdirAll(filepath.Dir(si.Path), 0o750)
	data, _ := json.MarshalIndent(si.Entries, "", "  ")
	_ = os.WriteFile(si.Path, data, 0o600)
}

// LearnPrompt generates a prompt that asks a model to extract a lesson from a
// failed interaction. It is the canonical instruction block used by the
// engine's Reflector (via buildReflectionPrompt) and surfaced by the
// `hawk learn` CLI. The response format matches parseReflectionEntry.
func LearnPrompt(context string) string {
	prompt := "A task just failed or produced a suboptimal result. Extract a lesson.\n\n"
	if context != "" {
		prompt += "Context: " + context + "\n\n"
	}
	prompt += "Respond with exactly three labeled lines:\n" +
		"WHAT_FAILED: <what went wrong, one sentence>\n" +
		"WHY_FAILED: <root cause>\n" +
		"WHAT_TO_DO: <what to do differently next time>"
	return prompt
}
