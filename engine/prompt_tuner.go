package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// PromptTuner optimizes system prompt sections based on session outcomes.
// Tracks which prompt configurations lead to successful sessions and
// adjusts over time (OPRO/EvoPrompt pattern without LLM calls).
type PromptTuner struct {
	mu       sync.Mutex
	variants []PromptVariant
	path     string
}

// PromptVariant is a tracked prompt configuration with its performance score.
type PromptVariant struct {
	Section   string    `json:"section"`   // which section was varied
	Content   string    `json:"content"`   // the variant content
	Score     float64   `json:"score"`     // success rate (0-1)
	Uses      int       `json:"uses"`      // times used
	Successes int       `json:"successes"` // successful sessions with this variant
	LastUsed  time.Time `json:"last_used"`
}

// NewPromptTuner creates a tuner backed by ~/.hawk/prompt_tuning.json.
func NewPromptTuner() *PromptTuner {
	home, _ := os.UserHomeDir()
	pt := &PromptTuner{
		path: filepath.Join(home, ".hawk", "prompt_tuning.json"),
	}
	pt.load()
	return pt
}

// RecordOutcome updates the variant score based on a session outcome.
func (pt *PromptTuner) RecordOutcome(section, content string, success bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	for i, v := range pt.variants {
		if v.Section == section && v.Content == content {
			pt.variants[i].Uses++
			if success {
				pt.variants[i].Successes++
			}
			pt.variants[i].Score = float64(pt.variants[i].Successes) / float64(pt.variants[i].Uses)
			pt.variants[i].LastUsed = time.Now()
			pt.save()
			return
		}
	}

	// New variant
	s := 0
	if success {
		s = 1
	}
	pt.variants = append(pt.variants, PromptVariant{
		Section:   section,
		Content:   content,
		Score:     float64(s),
		Uses:      1,
		Successes: s,
		LastUsed:  time.Now(),
	})
	pt.save()
}

// BestVariant returns the highest-scoring variant for a section.
func (pt *PromptTuner) BestVariant(section string) (string, float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	var best *PromptVariant
	for i, v := range pt.variants {
		if v.Section != section || v.Uses < 3 {
			continue
		}
		if best == nil || v.Score > best.Score {
			best = &pt.variants[i]
		}
	}
	if best != nil {
		return best.Content, best.Score
	}
	return "", 0
}

// Report returns a summary of all tracked variants sorted by score.
func (pt *PromptTuner) Report() string {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	sorted := make([]PromptVariant, len(pt.variants))
	copy(sorted, pt.variants)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })

	var b strings.Builder
	b.WriteString("## Prompt Tuning Report\n")
	for _, v := range sorted {
		if v.Uses < 2 {
			continue
		}
		b.WriteString("  " + v.Section + ": score=" + formatFloat(v.Score) + " (" + itoa2(v.Successes) + "/" + itoa2(v.Uses) + ")\n")
	}
	return b.String()
}

func (pt *PromptTuner) load() {
	data, err := os.ReadFile(pt.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &pt.variants)
}

func (pt *PromptTuner) save() {
	dir := filepath.Dir(pt.path)
	os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(pt.variants)
	os.WriteFile(pt.path, data, 0o644)
}

func formatFloat(f float64) string {
	s := ""
	whole := int(f * 100)
	s = itoa2(whole/100) + "." + itoa2(whole%100)
	return s
}

func itoa2(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
