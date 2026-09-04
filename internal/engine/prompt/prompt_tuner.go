package prompt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

type PromptTuner struct {
	mu       sync.Mutex
	variants []PromptVariant
	path     string
}

type PromptVariant struct {
	Section   string    `json:"section"`
	Content   string    `json:"content"`
	Score     float64   `json:"score"`
	Uses      int       `json:"uses"`
	Successes int       `json:"successes"`
	LastUsed  time.Time `json:"last_used"`
}

func NewPromptTuner() *PromptTuner {
	pt := &PromptTuner{
		path: filepath.Join(storage.StateDir(), "prompt_tuning.json"),
	}
	pt.load()
	return pt
}

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
	_ = json.Unmarshal(data, &pt.variants)
}

func (pt *PromptTuner) save() {
	dir := filepath.Dir(pt.path)
	_ = os.MkdirAll(dir, 0o750)
	data, _ := json.Marshal(pt.variants)
	_ = os.WriteFile(pt.path, data, 0o600)
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
