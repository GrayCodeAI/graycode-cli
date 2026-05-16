package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SelfImproveEntry records a lesson learned from a mistake.
type SelfImproveEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	What        string    `json:"what"`        // what went wrong
	Why         string    `json:"why"`         // root cause
	Lesson      string    `json:"lesson"`      // what to do differently
	Category    string    `json:"category"`    // code, test, design, communication
}

// SelfImprover tracks mistakes and lessons across sessions.
type SelfImprover struct {
	Path    string
	Entries []SelfImproveEntry
}

// NewSelfImprover loads or creates the improvement log.
func NewSelfImprover() *SelfImprover {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".hawk", "self-improve.json")
	si := &SelfImprover{Path: path}
	si.load()
	return si
}

// Learn records a new lesson.
func (si *SelfImprover) Learn(what, why, lesson, category string) {
	si.Entries = append(si.Entries, SelfImproveEntry{
		Timestamp: time.Now(),
		What:      what,
		Why:       why,
		Lesson:    lesson,
		Category:  category,
	})
	si.save()
}

// Lessons returns all lessons, optionally filtered by category.
func (si *SelfImprover) Lessons(category string) []SelfImproveEntry {
	if category == "" {
		return si.Entries
	}
	var filtered []SelfImproveEntry
	for _, e := range si.Entries {
		if e.Category == category {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// ForPrompt formats recent lessons as context for the system prompt.
func (si *SelfImprover) ForPrompt(maxEntries int) string {
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
	json.Unmarshal(data, &si.Entries)
}

func (si *SelfImprover) save() {
	os.MkdirAll(filepath.Dir(si.Path), 0o755)
	data, _ := json.MarshalIndent(si.Entries, "", "  ")
	os.WriteFile(si.Path, data, 0o644)
}

// LearnPrompt generates a prompt to extract lessons from a failed interaction.
func LearnPrompt(context string) string {
	return `A task just failed or produced a suboptimal result. Extract a lesson.

Context: ` + context + `

Respond with:
- **What went wrong:** (one sentence)
- **Why:** (root cause)
- **Lesson:** (what to do differently next time)
- **Category:** code | test | design | communication`
}
