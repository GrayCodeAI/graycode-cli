package errs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/hawk/internal/home"
)

type ErrorPattern struct {
	Trigger    string    `json:"trigger"`
	RootCause  string    `json:"root_cause"`
	Resolution string    `json:"resolution"`
	HitCount   int       `json:"hit_count"`
	LastSeen   time.Time `json:"last_seen"`
}

type ErrorPatternDB struct {
	mu       sync.Mutex
	patterns []ErrorPattern
	path     string
}

func NewErrorPatternDB() *ErrorPatternDB {
	home := home.Dir()
	db := &ErrorPatternDB{
		path: filepath.Join(home, ".hawk", "error_patterns.json"),
	}
	db.load()
	return db
}

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
	_ = json.Unmarshal(data, &db.patterns)
}

func (db *ErrorPatternDB) save() {
	dir := filepath.Dir(db.path)
	_ = os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(db.patterns)
	_ = os.WriteFile(db.path, data, 0o644)
}
