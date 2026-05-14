// Package staleness detects rules/skills that are no longer actively used
// or that contradict observed user behavior.
package staleness

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// StaleRule represents a rule that may no longer be relevant.
type StaleRule struct {
	ID                string
	Path              string
	LastUsed          time.Time
	DaysSinceUsed     int
	ContradictionCount int
	Contradictions    []Contradiction
}

// Contradiction records when a user consistently does the opposite of what a rule says.
type Contradiction struct {
	RuleID       string
	UserBehavior string
	Timestamp    time.Time
}

// RuleUsage tracks when a rule was last used.
type RuleUsage struct {
	RuleID    string
	Path      string
	LastUsed  time.Time
	UseCount  int
}

// Detector tracks which rules have influenced agent behavior and identifies stale ones.
type Detector struct {
	mu             sync.RWMutex
	usages         map[string]*RuleUsage
	contradictions map[string][]Contradiction
}

// NewDetector creates a new staleness detector.
func NewDetector() *Detector {
	return &Detector{
		usages:         make(map[string]*RuleUsage),
		contradictions: make(map[string][]Contradiction),
	}
}

// RecordRuleUsed marks a rule as having been used at the given time.
func (d *Detector) RecordRuleUsed(ruleID string, timestamp time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()

	usage, ok := d.usages[ruleID]
	if !ok {
		d.usages[ruleID] = &RuleUsage{
			RuleID:   ruleID,
			LastUsed: timestamp,
			UseCount: 1,
		}
		return
	}

	if timestamp.After(usage.LastUsed) {
		usage.LastUsed = timestamp
	}
	usage.UseCount++
}

// RecordRulePath associates a filesystem path with a rule ID.
func (d *Detector) RecordRulePath(ruleID, path string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	usage, ok := d.usages[ruleID]
	if !ok {
		d.usages[ruleID] = &RuleUsage{
			RuleID: ruleID,
			Path:   path,
		}
		return
	}
	usage.Path = path
}

// RecordContradiction records when user behavior contradicts what a rule prescribes.
func (d *Detector) RecordContradiction(ruleID string, userBehavior string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	c := Contradiction{
		RuleID:       ruleID,
		UserBehavior: userBehavior,
		Timestamp:    time.Now(),
	}
	d.contradictions[ruleID] = append(d.contradictions[ruleID], c)
}

// CheckStaleness returns rules that haven't been used within the threshold duration.
func (d *Detector) CheckStaleness(threshold time.Duration) []StaleRule {
	d.mu.RLock()
	defer d.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-threshold)
	var stale []StaleRule

	for id, usage := range d.usages {
		if usage.LastUsed.IsZero() || usage.LastUsed.Before(cutoff) {
			days := 0
			if !usage.LastUsed.IsZero() {
				days = int(now.Sub(usage.LastUsed).Hours() / 24)
			}

			contradictions := d.contradictions[id]
			sr := StaleRule{
				ID:                 id,
				Path:               usage.Path,
				LastUsed:           usage.LastUsed,
				DaysSinceUsed:      days,
				ContradictionCount: len(contradictions),
				Contradictions:     contradictions,
			}
			stale = append(stale, sr)
		}
	}

	// Also include rules with high contradiction count regardless of usage time.
	for id, contradictions := range d.contradictions {
		if len(contradictions) >= 3 {
			// Check if already included.
			found := false
			for _, s := range stale {
				if s.ID == id {
					found = true
					break
				}
			}
			if !found {
				usage := d.usages[id]
				path := ""
				var lastUsed time.Time
				if usage != nil {
					path = usage.Path
					lastUsed = usage.LastUsed
				}
				stale = append(stale, StaleRule{
					ID:                 id,
					Path:               path,
					LastUsed:           lastUsed,
					DaysSinceUsed:      int(now.Sub(lastUsed).Hours() / 24),
					ContradictionCount: len(contradictions),
					Contradictions:     contradictions,
				})
			}
		}
	}

	sort.Slice(stale, func(i, j int) bool {
		// Sort by staleness: most stale first.
		if stale[i].DaysSinceUsed != stale[j].DaysSinceUsed {
			return stale[i].DaysSinceUsed > stale[j].DaysSinceUsed
		}
		return stale[i].ContradictionCount > stale[j].ContradictionCount
	})

	return stale
}

// AllUsages returns all tracked rule usages.
func (d *Detector) AllUsages() map[string]*RuleUsage {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]*RuleUsage, len(d.usages))
	for k, v := range d.usages {
		copy := *v
		result[k] = &copy
	}
	return result
}

// FormatReport generates a human-readable staleness report.
func FormatReport(stale []StaleRule) string {
	if len(stale) == 0 {
		return "No stale rules detected. All rules are actively used."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Stale Rules Report (%d rules)\n", len(stale)))
	b.WriteString(strings.Repeat("-", 50) + "\n\n")

	for _, sr := range stale {
		b.WriteString(fmt.Sprintf("Rule: %s\n", sr.ID))
		if sr.Path != "" {
			b.WriteString(fmt.Sprintf("  Path: %s\n", sr.Path))
		}
		if !sr.LastUsed.IsZero() {
			b.WriteString(fmt.Sprintf("  Last used: %s (%d days ago)\n", sr.LastUsed.Format("2006-01-02"), sr.DaysSinceUsed))
		} else {
			b.WriteString("  Last used: never\n")
		}
		if sr.ContradictionCount > 0 {
			b.WriteString(fmt.Sprintf("  Contradictions: %d\n", sr.ContradictionCount))
			for _, c := range sr.Contradictions {
				b.WriteString(fmt.Sprintf("    - %s (%s)\n", c.UserBehavior, c.Timestamp.Format("2006-01-02")))
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// RecommendAction suggests what to do with a stale rule.
func RecommendAction(sr StaleRule) string {
	switch {
	case sr.ContradictionCount >= 5:
		return "REMOVE: Rule actively contradicts user behavior"
	case sr.ContradictionCount >= 3:
		return "REVIEW: Rule frequently contradicts user behavior"
	case sr.DaysSinceUsed >= 30:
		return "REMOVE: Rule unused for over a month"
	case sr.DaysSinceUsed >= 14:
		return "REVIEW: Rule unused for over two weeks"
	default:
		return "MONITOR: Rule may be becoming stale"
	}
}
