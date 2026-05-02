package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TransferLearning enables cross-session knowledge transfer.
// Extracts generalizable patterns from completed sessions and applies them
// to new sessions on similar codebases.
type TransferLearning struct {
	mu       sync.Mutex
	patterns []TransferPattern
	path     string
}

// TransferPattern is a reusable pattern extracted from a successful session.
type TransferPattern struct {
	Language    string    `json:"language"`    // go, python, typescript, etc.
	Category    string    `json:"category"`    // "fix", "refactor", "feature", "test"
	Pattern     string    `json:"pattern"`     // generalized description
	Approach    string    `json:"approach"`    // what worked
	Confidence  float64   `json:"confidence"`  // based on success rate
	UsedCount   int       `json:"used_count"`
	SuccessRate float64   `json:"success_rate"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewTransferLearning creates a store backed by ~/.hawk/transfer.json.
func NewTransferLearning() *TransferLearning {
	home, _ := os.UserHomeDir()
	tl := &TransferLearning{
		path: filepath.Join(home, ".hawk", "transfer.json"),
	}
	tl.load()
	return tl
}

// Learn extracts a transferable pattern from a successful session.
func (tl *TransferLearning) Learn(language, category, pattern, approach string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	// Check for existing similar pattern
	for i, p := range tl.patterns {
		if p.Language == language && p.Category == category &&
			strings.Contains(strings.ToLower(p.Pattern), strings.ToLower(pattern)) {
			tl.patterns[i].UsedCount++
			tl.patterns[i].SuccessRate = (tl.patterns[i].SuccessRate*float64(tl.patterns[i].UsedCount-1) + 1.0) / float64(tl.patterns[i].UsedCount)
			tl.patterns[i].Confidence = tl.patterns[i].SuccessRate
			tl.save()
			return
		}
	}

	tl.patterns = append(tl.patterns, TransferPattern{
		Language:    language,
		Category:    category,
		Pattern:     pattern,
		Approach:    approach,
		Confidence:  0.6,
		UsedCount:   1,
		SuccessRate: 1.0,
		CreatedAt:   time.Now(),
	})
	tl.save()
}

// Apply finds patterns relevant to the current task.
func (tl *TransferLearning) Apply(language, taskDescription string) []TransferPattern {
	tl.mu.Lock()
	defer tl.mu.Unlock()

	taskLower := strings.ToLower(taskDescription)
	var matches []TransferPattern

	for _, p := range tl.patterns {
		if p.Confidence < 0.5 {
			continue
		}
		// Language match (or language-agnostic)
		if p.Language != "" && p.Language != language {
			continue
		}
		// Keyword overlap
		words := strings.Fields(strings.ToLower(p.Pattern))
		matchCount := 0
		for _, w := range words {
			if len(w) > 3 && strings.Contains(taskLower, w) {
				matchCount++
			}
		}
		if matchCount > 0 || strings.Contains(taskLower, p.Category) {
			matches = append(matches, p)
		}
	}

	// Cap at 3 most confident
	if len(matches) > 3 {
		for i := 0; i < len(matches); i++ {
			for j := i + 1; j < len(matches); j++ {
				if matches[j].Confidence > matches[i].Confidence {
					matches[i], matches[j] = matches[j], matches[i]
				}
			}
		}
		matches = matches[:3]
	}
	return matches
}

// FormatForPrompt returns transfer patterns as system prompt context.
func (tl *TransferLearning) FormatForPrompt(language, task string) string {
	patterns := tl.Apply(language, task)
	if len(patterns) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Learned Patterns (from prior sessions)\n")
	for _, p := range patterns {
		b.WriteString("- " + p.Pattern + " → " + p.Approach + "\n")
	}
	return b.String()
}

func (tl *TransferLearning) load() {
	data, err := os.ReadFile(tl.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &tl.patterns)
}

func (tl *TransferLearning) save() {
	dir := filepath.Dir(tl.path)
	os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(tl.patterns)
	os.WriteFile(tl.path, data, 0o644)
}
