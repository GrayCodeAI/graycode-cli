package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"sync"
	"time"
)

// LearnedPattern represents a learned error pattern with its fix information.
type LearnedPattern struct {
	ID           string    `json:"id"`
	Category     string    `json:"category"`     // "build", "test", "lint", "runtime"
	Language     string    `json:"language"`     // "go", "python", "js", "ts", "rust", "generic"
	Pattern      string    `json:"pattern"`      // regex that matches this error
	Example      string    `json:"example"`      // actual error message example
	Fix          string    `json:"fix"`          // description of how to fix
	FixTemplate  string    `json:"fix_template"` // template for automated fix
	Confidence   float64   `json:"confidence"`   // how reliable this fix is (0-1)
	SuccessCount int       `json:"success_count"`
	FailureCount int       `json:"failure_count"`
	LastSeen     time.Time `json:"last_seen"`
}

// ErrorLearnerStats holds aggregate statistics about the error learner.
type ErrorLearnerStats struct {
	TotalPatterns int            `json:"total_patterns"`
	ByCategory    map[string]int `json:"by_category"`
	ByLanguage    map[string]int `json:"by_language"`
	AvgConfidence float64        `json:"avg_confidence"`
}

// ErrorLearner stores and matches error patterns to provide fix suggestions.
type ErrorLearner struct {
	Patterns map[string]*LearnedPattern
	mu       sync.RWMutex
}

// NewErrorLearner creates an ErrorLearner pre-loaded with common error patterns.
func NewErrorLearner() *ErrorLearner {
	el := &ErrorLearner{
		Patterns: make(map[string]*LearnedPattern),
	}
	el.loadDefaults()
	return el
}

func (el *ErrorLearner) loadDefaults() {
	defaults := []*LearnedPattern{
		{
			ID:          "go-undefined",
			Category:    "build",
			Language:    "go",
			Pattern:     `undefined:\s*\w+`,
			Example:     "undefined: handleAuth",
			Fix:         "Add missing function declaration or import the package that provides it.",
			FixTemplate: "// Add declaration for {{.Name}} or import the providing package",
			Confidence:  0.85,
		},
		{
			ID:          "go-type-mismatch",
			Category:    "build",
			Language:    "go",
			Pattern:     `cannot use .* as type`,
			Example:     "cannot use x (variable of type int) as type string",
			Fix:         "Convert the value to the expected type or change the variable declaration.",
			FixTemplate: "// Convert value to expected type",
			Confidence:  0.80,
		},
		{
			ID:          "go-missing-args",
			Category:    "build",
			Language:    "go",
			Pattern:     `not enough arguments`,
			Example:     "not enough arguments in call to fmt.Fprintf",
			Fix:         "Add the missing arguments to the function call.",
			FixTemplate: "// Add missing arguments to function call",
			Confidence:  0.82,
		},
		{
			ID:          "go-unused-var",
			Category:    "build",
			Language:    "go",
			Pattern:     `declared (and|but) not used`,
			Example:     "x declared and not used",
			Fix:         "Remove the unused variable or use it in the code.",
			FixTemplate: "_ = {{.Name}} // use or remove the variable",
			Confidence:  0.95,
		},
		{
			ID:          "go-unused-import",
			Category:    "build",
			Language:    "go",
			Pattern:     `imported and not used`,
			Example:     `"fmt" imported and not used`,
			Fix:         "Remove the unused import or use a blank identifier.",
			FixTemplate: "// Remove the unused import",
			Confidence:  0.95,
		},
		{
			ID:          "python-indentation",
			Category:    "build",
			Language:    "python",
			Pattern:     `IndentationError`,
			Example:     "IndentationError: unexpected indent",
			Fix:         "Fix the indentation to match the surrounding code block.",
			FixTemplate: "# Fix indentation to use consistent spaces/tabs",
			Confidence:  0.88,
		},
		{
			ID:          "python-name-error",
			Category:    "runtime",
			Language:    "python",
			Pattern:     `NameError: name .* is not defined`,
			Example:     "NameError: name 'foo' is not defined",
			Fix:         "Define the variable before use or import the module that provides it.",
			FixTemplate: "# Define or import {{.Name}}",
			Confidence:  0.82,
		},
		{
			ID:          "js-missing-module",
			Category:    "build",
			Language:    "js",
			Pattern:     `Cannot find module`,
			Example:     "Cannot find module './utils'",
			Fix:         "Install the missing package or fix the import path.",
			FixTemplate: "// npm install <package> or fix import path",
			Confidence:  0.85,
		},
		{
			ID:          "ts-type-assignable",
			Category:    "build",
			Language:    "ts",
			Pattern:     `is not assignable to type`,
			Example:     "Type 'string' is not assignable to type 'number'",
			Fix:         "Fix the type mismatch by converting the value or updating the type annotation.",
			FixTemplate: "// Fix type to match expected",
			Confidence:  0.80,
		},
		{
			ID:          "rust-undefined",
			Category:    "build",
			Language:    "rust",
			Pattern:     `cannot find value`,
			Example:     "cannot find value `x` in this scope",
			Fix:         "Declare the variable or bring it into scope with use.",
			FixTemplate: "// let {{.Name}} = ... or use the correct path",
			Confidence:  0.82,
		},
		{
			ID:          "generic-permission",
			Category:    "runtime",
			Language:    "generic",
			Pattern:     `permission denied`,
			Example:     "permission denied: /etc/shadow",
			Fix:         "Check file permissions and ensure the process has appropriate access.",
			FixTemplate: "// chmod or run with appropriate permissions",
			Confidence:  0.75,
		},
		{
			ID:          "generic-connection-refused",
			Category:    "runtime",
			Language:    "generic",
			Pattern:     `connection refused`,
			Example:     "dial tcp 127.0.0.1:5432: connection refused",
			Fix:         "Ensure the target service is running and listening on the expected port.",
			FixTemplate: "// Start the service or check the port configuration",
			Confidence:  0.78,
		},
	}

	for _, p := range defaults {
		p.LastSeen = time.Now()
		el.Patterns[p.ID] = p
	}
}

// MatchLearned finds all learned patterns matching the error message, sorted by confidence (highest first).
func (el *ErrorLearner) MatchLearned(errorMsg string) []*LearnedPattern {
	el.mu.RLock()
	defer el.mu.RUnlock()

	var matches []*LearnedPattern
	for _, p := range el.Patterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			continue
		}
		if re.MatchString(errorMsg) {
			p.LastSeen = time.Now()
			matches = append(matches, p)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Confidence > matches[j].Confidence
	})

	return matches
}

// Learn records a new error pattern or updates an existing one.
func (el *ErrorLearner) Learn(errorMsg, fix, language, category string) {
	el.mu.Lock()
	defer el.mu.Unlock()

	// Check if error matches an existing pattern.
	for _, p := range el.Patterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			continue
		}
		if re.MatchString(errorMsg) {
			p.SuccessCount++
			p.LastSeen = time.Now()
			if fix != "" {
				p.Fix = fix
			}
			return
		}
	}

	// Create new pattern.
	pattern := ExtractPattern(errorMsg)
	id := fmt.Sprintf("%s-%s-%d", language, category, time.Now().UnixNano())

	el.Patterns[id] = &LearnedPattern{
		ID:           id,
		Category:     category,
		Language:     language,
		Pattern:      pattern,
		Example:      errorMsg,
		Fix:          fix,
		Confidence:   0.5,
		SuccessCount: 0,
		FailureCount: 0,
		LastSeen:     time.Now(),
	}
}

// RecordSuccess increments the success count and boosts confidence for a pattern.
func (el *ErrorLearner) RecordSuccess(patternID string) {
	el.mu.Lock()
	defer el.mu.Unlock()

	p, ok := el.Patterns[patternID]
	if !ok {
		return
	}
	p.SuccessCount++
	p.LastSeen = time.Now()
	total := float64(p.SuccessCount + p.FailureCount)
	if total > 0 {
		p.Confidence = float64(p.SuccessCount) / total
	}
}

// RecordFailure increments the failure count and reduces confidence for a pattern.
func (el *ErrorLearner) RecordFailure(patternID string) {
	el.mu.Lock()
	defer el.mu.Unlock()

	p, ok := el.Patterns[patternID]
	if !ok {
		return
	}
	p.FailureCount++
	p.LastSeen = time.Now()
	total := float64(p.SuccessCount + p.FailureCount)
	if total > 0 {
		p.Confidence = float64(p.SuccessCount) / total
	}
}

// BuildFixSuggestion matches the error and returns a formatted fix suggestion.
func (el *ErrorLearner) BuildFixSuggestion(errorMsg string) string {
	matches := el.MatchLearned(errorMsg)
	if len(matches) == 0 {
		return ""
	}

	best := matches[0]
	confidencePct := int(best.Confidence * 100)

	return fmt.Sprintf(
		"Known error pattern: %q\nCategory: %s (%s)\nSuggested fix: %s\nConfidence: %d%% (%d successes, %d failures)",
		errorMsg,
		best.Category,
		best.Language,
		best.Fix,
		confidencePct,
		best.SuccessCount,
		best.FailureCount,
	)
}

// ExtractPattern generalizes a specific error message into a regex pattern.
func ExtractPattern(errorMsg string) string {
	// Escape regex metacharacters first.
	escaped := regexp.QuoteMeta(errorMsg)

	// Replace file paths (sequences with slashes).
	pathRe := regexp.MustCompile(`[^\s\\]*(/[^\s\\]+)+`)
	result := pathRe.ReplaceAllString(escaped, `[^\s]+`)

	// Replace quoted identifiers.
	quotedRe := regexp.MustCompile(`\\['"]\\?\w+\\?['"]`)
	result = quotedRe.ReplaceAllString(result, `\w+`)

	// Replace numbers.
	numRe := regexp.MustCompile(`\d+`)
	result = numRe.ReplaceAllString(result, `\d+`)

	// Replace standalone identifiers that look like variable/function names
	// (camelCase or snake_case words not part of the error keyword text).
	identRe := regexp.MustCompile(`\b[a-z][a-zA-Z_]*[A-Z]\w*\b`)
	result = identRe.ReplaceAllString(result, `\w+`)

	return result
}

// PruneWeak removes patterns with confidence below the given threshold.
func (el *ErrorLearner) PruneWeak(minConfidence float64) {
	el.mu.Lock()
	defer el.mu.Unlock()

	for id, p := range el.Patterns {
		if p.Confidence < minConfidence {
			delete(el.Patterns, id)
		}
	}
}

// Export serializes all patterns to JSON for persistence.
func (el *ErrorLearner) Export() ([]byte, error) {
	el.mu.RLock()
	defer el.mu.RUnlock()

	return json.MarshalIndent(el.Patterns, "", "  ")
}

// Import loads patterns from JSON data, merging with existing patterns.
func (el *ErrorLearner) Import(data []byte) error {
	el.mu.Lock()
	defer el.mu.Unlock()

	var patterns map[string]*LearnedPattern
	if err := json.Unmarshal(data, &patterns); err != nil {
		return fmt.Errorf("import error patterns: %w", err)
	}

	for id, p := range patterns {
		el.Patterns[id] = p
	}
	return nil
}

// Stats returns aggregate statistics about the learner's patterns.
func (el *ErrorLearner) Stats() ErrorLearnerStats {
	el.mu.RLock()
	defer el.mu.RUnlock()

	stats := ErrorLearnerStats{
		TotalPatterns: len(el.Patterns),
		ByCategory:    make(map[string]int),
		ByLanguage:    make(map[string]int),
	}

	var totalConfidence float64
	for _, p := range el.Patterns {
		stats.ByCategory[p.Category]++
		stats.ByLanguage[p.Language]++
		totalConfidence += p.Confidence
	}

	if stats.TotalPatterns > 0 {
		stats.AvgConfidence = totalConfidence / float64(stats.TotalPatterns)
	}

	return stats
}
