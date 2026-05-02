package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrorPattern records a known error trigger, root cause, and resolution.
type ErrorPattern struct {
	Trigger    string    `json:"trigger"`     // error message pattern
	RootCause  string    `json:"root_cause"`  // why it happens
	Resolution string    `json:"resolution"`  // how to fix it
	HitCount   int       `json:"hit_count"`   // times encountered
	LastSeen   time.Time `json:"last_seen"`
}

// ErrorPatternDB learns from tool failures and prevents repeating mistakes.
type ErrorPatternDB struct {
	mu       sync.Mutex
	patterns []ErrorPattern
	path     string
}

// NewErrorPatternDB creates a database backed by ~/.hawk/error_patterns.json.
func NewErrorPatternDB() *ErrorPatternDB {
	home, _ := os.UserHomeDir()
	db := &ErrorPatternDB{
		path: filepath.Join(home, ".hawk", "error_patterns.json"),
	}
	db.load()
	return db
}

// Record adds or updates an error pattern.
func (db *ErrorPatternDB) Record(trigger, rootCause, resolution string) {
	db.mu.Lock()
	defer db.mu.Unlock()

	triggerLower := strings.ToLower(trigger)

	for i, p := range db.patterns {
		if strings.Contains(strings.ToLower(p.Trigger), triggerLower) ||
			strings.Contains(triggerLower, strings.ToLower(p.Trigger)) {
			db.patterns[i].HitCount++
			db.patterns[i].LastSeen = time.Now()
			if resolution != "" {
				db.patterns[i].Resolution = resolution
			}
			db.save()
			return
		}
	}

	db.patterns = append(db.patterns, ErrorPattern{
		Trigger:    trigger,
		RootCause:  rootCause,
		Resolution: resolution,
		HitCount:   1,
		LastSeen:   time.Now(),
	})
	db.save()
}

// Match finds patterns that match the given error message.
func (db *ErrorPatternDB) Match(errorMsg string) []ErrorPattern {
	db.mu.Lock()
	defer db.mu.Unlock()

	errorLower := strings.ToLower(errorMsg)
	var matches []ErrorPattern

	for _, p := range db.patterns {
		if strings.Contains(errorLower, strings.ToLower(p.Trigger)) {
			matches = append(matches, p)
		}
	}
	return matches
}

// FormatHints returns actionable hints for an error from known patterns.
func (db *ErrorPatternDB) FormatHints(errorMsg string) string {
	matches := db.Match(errorMsg)
	if len(matches) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Known Error Patterns\n")
	for _, p := range matches {
		b.WriteString("- " + p.Trigger + "\n")
		if p.RootCause != "" {
			b.WriteString("  Cause: " + p.RootCause + "\n")
		}
		if p.Resolution != "" {
			b.WriteString("  Fix: " + p.Resolution + "\n")
		}
	}
	return b.String()
}

func (db *ErrorPatternDB) load() {
	data, err := os.ReadFile(db.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &db.patterns)
}

func (db *ErrorPatternDB) save() {
	dir := filepath.Dir(db.path)
	os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(db.patterns)
	os.WriteFile(db.path, data, 0o644)
}
