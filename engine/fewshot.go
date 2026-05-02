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

// FewShotStore collects successful (prompt, response) pairs and injects
// the most relevant as few-shot examples into the system prompt.
type FewShotStore struct {
	mu       sync.Mutex
	examples []FewShotExample
	path     string
	maxStore int
}

// FewShotExample is a recorded successful interaction.
type FewShotExample struct {
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response"`
	TaskType  string    `json:"task_type"`
	Quality   float64   `json:"quality"` // 0-1, based on whether output was kept
	CreatedAt time.Time `json:"created_at"`
	UsedCount int       `json:"used_count"`
}

// NewFewShotStore creates a store backed by ~/.hawk/fewshot.json.
func NewFewShotStore() *FewShotStore {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".hawk", "fewshot.json")
	fs := &FewShotStore{
		path:     path,
		maxStore: 50,
	}
	fs.load()
	return fs
}

// Record saves a successful interaction as a potential few-shot example.
func (fs *FewShotStore) Record(prompt, response, taskType string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Only store non-trivial examples
	if len(prompt) < 20 || len(response) < 50 {
		return
	}

	// Truncate for storage efficiency
	if len(prompt) > 500 {
		prompt = prompt[:500]
	}
	if len(response) > 1000 {
		response = response[:1000]
	}

	fs.examples = append(fs.examples, FewShotExample{
		Prompt:    prompt,
		Response:  response,
		TaskType:  taskType,
		Quality:   1.0,
		CreatedAt: time.Now(),
	})

	// Keep only the best examples
	if len(fs.examples) > fs.maxStore {
		sort.Slice(fs.examples, func(i, j int) bool {
			return fs.examples[i].Quality > fs.examples[j].Quality
		})
		fs.examples = fs.examples[:fs.maxStore]
	}

	fs.save()
}

// Retrieve finds the most relevant few-shot examples for a given prompt.
func (fs *FewShotStore) Retrieve(prompt string, topK int) []FewShotExample {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if len(fs.examples) == 0 {
		return nil
	}

	type scored struct {
		example FewShotExample
		score   float64
	}

	lowerPrompt := strings.ToLower(prompt)
	var candidates []scored

	for _, ex := range fs.examples {
		// Simple keyword overlap scoring
		words := strings.Fields(strings.ToLower(ex.Prompt))
		matches := 0
		for _, w := range words {
			if len(w) > 3 && strings.Contains(lowerPrompt, w) {
				matches++
			}
		}
		if matches == 0 {
			continue
		}
		score := float64(matches) / float64(len(words)+1)
		score *= ex.Quality
		candidates = append(candidates, scored{ex, score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if topK > len(candidates) {
		topK = len(candidates)
	}

	out := make([]FewShotExample, topK)
	for i := 0; i < topK; i++ {
		out[i] = candidates[i].example
		// Track usage
		for j := range fs.examples {
			if fs.examples[j].Prompt == out[i].Prompt {
				fs.examples[j].UsedCount++
			}
		}
	}
	return out
}

// FormatForPrompt returns few-shot examples formatted for system prompt injection.
func (fs *FewShotStore) FormatForPrompt(prompt string) string {
	examples := fs.Retrieve(prompt, 3)
	if len(examples) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Successful Examples from Prior Sessions\n")
	for _, ex := range examples {
		b.WriteString("User: " + ex.Prompt + "\n")
		b.WriteString("Assistant: " + ex.Response + "\n\n")
	}
	return b.String()
}

func (fs *FewShotStore) load() {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &fs.examples)
}

func (fs *FewShotStore) save() {
	dir := filepath.Dir(fs.path)
	os.MkdirAll(dir, 0o755)
	data, _ := json.Marshal(fs.examples)
	os.WriteFile(fs.path, data, 0o644)
}
