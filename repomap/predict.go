package repomap

import (
	"path/filepath"
	"strings"
	"time"
)

// RelevancePrediction holds predicted files with their relevance scores.
type RelevancePrediction struct {
	Files []PredictedFile
}

// PredictedFile is a file predicted to be relevant to the current task.
type PredictedFile struct {
	Path      string
	Score     float64
	Reason    string // why it was predicted relevant
}

// PredictRelevantFiles predicts which files are likely relevant given:
// - The user's prompt (keyword matching against repo map symbols)
// - Recently edited files (locality heuristic)
// - Import graph relationships
// - Co-change history
func PredictRelevantFiles(prompt string, recentEdits []RecentEdit, graph *ImportGraph, symbols map[string]string) *RelevancePrediction {
	pred := &RelevancePrediction{}
	scored := make(map[string]float64)
	reasons := make(map[string]string)

	promptLower := strings.ToLower(prompt)
	promptWords := strings.Fields(promptLower)

	// Signal 1: Keyword match against symbol names -> file paths
	for symbol, filePath := range symbols {
		symbolLower := strings.ToLower(symbol)
		for _, word := range promptWords {
			if len(word) > 3 && strings.Contains(symbolLower, word) {
				scored[filePath] += 0.3
				if reasons[filePath] == "" {
					reasons[filePath] = "symbol match: " + symbol
				}
			}
		}
	}

	// Signal 2: Recently edited files (strong locality signal)
	for _, edit := range recentEdits {
		age := time.Since(edit.At)
		recencyBoost := 0.5
		if age > 5*time.Minute {
			recencyBoost = 0.3
		}
		if age > 30*time.Minute {
			recencyBoost = 0.1
		}
		scored[edit.Path] += recencyBoost
		if reasons[edit.Path] == "" {
			reasons[edit.Path] = "recently edited"
		}
	}

	// Signal 3: Import graph expansion (files related to high-scoring files)
	if graph != nil {
		for path, score := range scored {
			if score < 0.2 {
				continue
			}
			deps := graph.DependenciesOf(path, 1)
			for _, dep := range deps {
				scored[dep] += score * 0.4
				if reasons[dep] == "" {
					reasons[dep] = "imported by " + filepath.Base(path)
				}
			}
		}
	}

	// Signal 4: File path contains prompt keywords
	for _, word := range promptWords {
		if len(word) < 4 {
			continue
		}
		for path := range symbols {
			baseLower := strings.ToLower(filepath.Base(path))
			if strings.Contains(baseLower, word) {
				scored[path] += 0.2
				if reasons[path] == "" {
					reasons[path] = "filename match"
				}
			}
		}
	}

	// Convert to sorted list
	for path, score := range scored {
		if score >= 0.2 {
			pred.Files = append(pred.Files, PredictedFile{
				Path:   path,
				Score:  score,
				Reason: reasons[path],
			})
		}
	}

	// Sort by score descending
	for i := 0; i < len(pred.Files); i++ {
		for j := i + 1; j < len(pred.Files); j++ {
			if pred.Files[j].Score > pred.Files[i].Score {
				pred.Files[i], pred.Files[j] = pred.Files[j], pred.Files[i]
			}
		}
	}

	// Cap at top 10
	if len(pred.Files) > 10 {
		pred.Files = pred.Files[:10]
	}

	return pred
}

// RecentEdit tracks a file that was recently modified.
type RecentEdit struct {
	Path string
	At   time.Time
}

// RecentEditTracker maintains a list of recently edited files.
type RecentEditTracker struct {
	edits []RecentEdit
	max   int
}

// NewRecentEditTracker creates a tracker with a max capacity.
func NewRecentEditTracker(max int) *RecentEditTracker {
	if max <= 0 {
		max = 50
	}
	return &RecentEditTracker{max: max}
}

// Record adds a file edit event.
func (t *RecentEditTracker) Record(path string) {
	t.edits = append(t.edits, RecentEdit{Path: path, At: time.Now()})
	if len(t.edits) > t.max {
		t.edits = t.edits[len(t.edits)-t.max:]
	}
}

// Recent returns edits within the given duration.
func (t *RecentEditTracker) Recent(within time.Duration) []RecentEdit {
	cutoff := time.Now().Add(-within)
	var result []RecentEdit
	for _, e := range t.edits {
		if e.At.After(cutoff) {
			result = append(result, e)
		}
	}
	return result
}
