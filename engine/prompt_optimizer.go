package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/eyrie/client"
)

// PromptParameter is a tunable prompt component (like a neural network weight).
type PromptParameter struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Score   float64 `json:"score"`    // 0.0-1.0 performance score
	Version int    `json:"version"`
}

// PromptGradient is textual feedback on how to improve a prompt (like a gradient).
type PromptGradient struct {
	Parameter string `json:"parameter"`
	Feedback  string `json:"feedback"` // what went wrong
	Direction string `json:"direction"` // how to improve
	Magnitude float64 `json:"magnitude"` // 0.0-1.0 how much to change
}

// PromptOptimizer auto-improves hawk's prompts based on success/failure signals.
type PromptOptimizer struct {
	Parameters map[string]*PromptParameter
	History    []OptimizationStep
	Path       string
}

// OptimizationStep records one optimization iteration.
type OptimizationStep struct {
	Timestamp  time.Time       `json:"timestamp"`
	Parameter  string          `json:"parameter"`
	OldValue   string          `json:"old_value"`
	NewValue   string          `json:"new_value"`
	OldScore   float64         `json:"old_score"`
	NewScore   float64         `json:"new_score"`
	Gradient   PromptGradient  `json:"gradient"`
}

// NewPromptOptimizer creates an optimizer that persists to ~/.hawk/prompt_params.json.
func NewPromptOptimizer() *PromptOptimizer {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".hawk", "prompt_params.json")
	po := &PromptOptimizer{
		Parameters: make(map[string]*PromptParameter),
		Path:       path,
	}
	po.load()
	return po
}

// Register adds a tunable prompt parameter.
func (po *PromptOptimizer) Register(name, initialValue string) {
	if _, exists := po.Parameters[name]; !exists {
		po.Parameters[name] = &PromptParameter{Name: name, Value: initialValue, Score: 0.5, Version: 1}
	}
}

// Get returns the current optimized value of a parameter.
func (po *PromptOptimizer) Get(name string) string {
	if p, ok := po.Parameters[name]; ok {
		return p.Value
	}
	return ""
}

// RecordSuccess signals that the current prompts worked well.
func (po *PromptOptimizer) RecordSuccess(paramName string) {
	if p, ok := po.Parameters[paramName]; ok {
		// Exponential moving average toward 1.0
		p.Score = p.Score*0.8 + 1.0*0.2
		po.save()
	}
}

// RecordFailure signals that a prompt produced bad results.
func (po *PromptOptimizer) RecordFailure(paramName, feedback string) {
	if p, ok := po.Parameters[paramName]; ok {
		p.Score = p.Score*0.8 + 0.0*0.2
		po.save()
	}
}

// ComputeGradient generates a textual gradient (improvement direction) for a parameter.
func ComputeGradientPrompt(paramName, currentValue, feedback string, examples []string) string {
	var exampleSection string
	if len(examples) > 0 {
		exampleSection = "\n\nSuccessful examples for reference:\n" + strings.Join(examples, "\n---\n")
	}

	return fmt.Sprintf(`You are a prompt optimizer. A prompt parameter is underperforming.

PARAMETER: %s
CURRENT VALUE:
%s

FAILURE FEEDBACK:
%s
%s
TASK: Rewrite the parameter value to address the feedback. Keep the same intent but improve clarity, specificity, or correctness.

Respond with ONLY the improved prompt text, nothing else.`, paramName, currentValue, feedback, exampleSection)
}

// ApplyGradient updates a parameter with an optimized value.
func (po *PromptOptimizer) ApplyGradient(paramName, newValue string, gradient PromptGradient) {
	p, ok := po.Parameters[paramName]
	if !ok {
		return
	}
	step := OptimizationStep{
		Timestamp: time.Now(),
		Parameter: paramName,
		OldValue:  p.Value,
		NewValue:  newValue,
		OldScore:  p.Score,
		Gradient:  gradient,
	}
	p.Value = newValue
	p.Version++
	p.Score = 0.5 // reset score for new version
	po.History = append(po.History, step)
	po.save()
}

// NeedsOptimization returns parameters with scores below threshold.
func (po *PromptOptimizer) NeedsOptimization(threshold float64) []*PromptParameter {
	var weak []*PromptParameter
	for _, p := range po.Parameters {
		if p.Score < threshold {
			weak = append(weak, p)
		}
	}
	return weak
}

// OptimizePrompt is the main entry point — given a failing parameter, generate an improved version.
func OptimizePrompt(ctx context.Context, llm LLMClient, model string, po *PromptOptimizer, paramName, feedback string) (string, error) {
	p, ok := po.Parameters[paramName]
	if !ok {
		return "", fmt.Errorf("parameter %q not found", paramName)
	}

	prompt := ComputeGradientPrompt(paramName, p.Value, feedback, nil)
	msgs := []client.EyrieMessage{{Role: "user", Content: prompt}}
	resp, err := llm.Chat(ctx, msgs, client.ChatOptions{Model: model})
	if err != nil {
		return "", err
	}

	newValue := strings.TrimSpace(resp.Content)
	gradient := PromptGradient{
		Parameter: paramName,
		Feedback:  feedback,
		Direction: "improved based on failure feedback",
		Magnitude: 0.5,
	}
	po.ApplyGradient(paramName, newValue, gradient)
	return newValue, nil
}

// PromptFewShotSelector picks the best few-shot examples from a pool based on similarity.
type PromptFewShotSelector struct {
	Examples []PromptExample
}

// PromptExample is a single example for few-shot prompting in the optimizer.
type PromptExample struct {
	Input    string  `json:"input"`
	Output   string  `json:"output"`
	Score    float64 `json:"score"`
	Category string  `json:"category"`
}

// Select picks the top-k examples most relevant to the query.
func (fs *PromptFewShotSelector) Select(query string, k int) []PromptExample {
	if len(fs.Examples) == 0 || k <= 0 {
		return nil
	}

	queryWords := strings.Fields(strings.ToLower(query))

	type scored struct {
		example PromptExample
		score   float64
	}

	var results []scored
	for _, ex := range fs.Examples {
		exWords := strings.Fields(strings.ToLower(ex.Input))
		overlap := promptWordOverlap(queryWords, exWords)
		finalScore := overlap * (0.5 + ex.Score*0.5)
		results = append(results, scored{example: ex, score: finalScore})
	}

	for i := 0; i < k && i < len(results); i++ {
		maxIdx := i
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[maxIdx].score {
				maxIdx = j
			}
		}
		results[i], results[maxIdx] = results[maxIdx], results[i]
	}

	var selected []PromptExample
	for i := 0; i < k && i < len(results); i++ {
		selected = append(selected, results[i].example)
	}
	return selected
}

// FormatPromptExamples renders selected examples as prompt context.
func FormatPromptExamples(examples []PromptExample) string {
	if len(examples) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Examples\n")
	for i, ex := range examples {
		sb.WriteString(fmt.Sprintf("\n### Example %d\nInput: %s\nOutput: %s\n", i+1, ex.Input, ex.Output))
	}
	return sb.String()
}

func promptWordOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	bSet := make(map[string]bool)
	for _, w := range b {
		bSet[w] = true
	}
	matches := 0
	for _, w := range a {
		if bSet[w] {
			matches++
		}
	}
	return float64(matches) / float64(len(a))
}

func (po *PromptOptimizer) load() {
	data, err := os.ReadFile(po.Path)
	if err != nil {
		return
	}
	json.Unmarshal(data, &po.Parameters)
}

func (po *PromptOptimizer) save() {
	os.MkdirAll(filepath.Dir(po.Path), 0o755)
	data, _ := json.MarshalIndent(po.Parameters, "", "  ")
	os.WriteFile(po.Path, data, 0o644)
}
