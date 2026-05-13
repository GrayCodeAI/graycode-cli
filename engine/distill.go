package engine

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DistillExample represents a single successful interaction captured for fine-tuning distillation.
type DistillExample struct {
	ID                string    `json:"id"`
	SystemPrompt      string    `json:"system_prompt"`
	UserMessage       string    `json:"user_message"`
	AssistantResponse string    `json:"assistant_response"`
	ToolCalls         []string  `json:"tool_calls,omitempty"`
	Quality           float64   `json:"quality"`
	Model             string    `json:"model"`
	Tokens            int       `json:"tokens"`
	CreatedAt         time.Time `json:"created_at"`
	Tags              []string  `json:"tags,omitempty"`
}

// DistillStats holds aggregate statistics about the distillation pipeline.
type DistillStats struct {
	TotalExamples int            `json:"total_examples"`
	AvgQuality    float64        `json:"avg_quality"`
	ByModel       map[string]int `json:"by_model"`
	ByTag         map[string]int `json:"by_tag"`
	TotalTokens   int            `json:"total_tokens"`
	EstimatedCost float64        `json:"estimated_cost"`
}

// DistillationPipeline collects successful interactions as fine-tuning examples
// to distill expensive model behavior into cheaper models.
type DistillationPipeline struct {
	Examples    []DistillExample `json:"examples"`
	Dir         string           `json:"dir"`
	TargetModel string           `json:"target_model"`
	SourceModel string           `json:"source_model"`
	MinQuality  float64          `json:"min_quality"`
	mu          sync.RWMutex
}

// NewDistillationPipeline creates a new distillation pipeline that stores data in dir.
func NewDistillationPipeline(dir string) *DistillationPipeline {
	return &DistillationPipeline{
		Examples:    make([]DistillExample, 0),
		Dir:         dir,
		TargetModel: "claude-haiku",
		SourceModel: "",
		MinQuality:  0.7,
	}
}

// Capture records a successful interaction as a training example.
// Only keeps the example if quality >= MinQuality.
func (dp *DistillationPipeline) Capture(system, user, assistant string, toolCalls []string, quality float64, model string) {
	if quality < dp.MinQuality {
		return
	}

	dp.mu.Lock()
	defer dp.mu.Unlock()

	id := generateDistillID(system, user, assistant)
	tokens := estimateDistillTokens(system) + estimateDistillTokens(user) + estimateDistillTokens(assistant)

	example := DistillExample{
		ID:                id,
		SystemPrompt:      system,
		UserMessage:       user,
		AssistantResponse: assistant,
		ToolCalls:         toolCalls,
		Quality:           quality,
		Model:             model,
		Tokens:            tokens,
		CreatedAt:         time.Now(),
		Tags:              inferTags(user, assistant),
	}

	dp.Examples = append(dp.Examples, example)

	if dp.SourceModel == "" {
		dp.SourceModel = model
	}
}

// ExportJSONL exports examples as JSONL format suitable for fine-tuning.
// Each line: {"messages":[{"role":"system","content":"..."},{"role":"user","content":"..."},{"role":"assistant","content":"..."}]}
func (dp *DistillationPipeline) ExportJSONL(path string) error {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("distill: create dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("distill: create file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, ex := range dp.Examples {
		record := buildMessagesRecord(ex)
		if err := enc.Encode(record); err != nil {
			return fmt.Errorf("distill: encode: %w", err)
		}
	}

	return nil
}

// ExportOpenAI exports examples in OpenAI fine-tuning format.
// Format: {"messages":[{"role":"system","content":"..."},{"role":"user","content":"..."},{"role":"assistant","content":"..."}]}
func (dp *DistillationPipeline) ExportOpenAI(path string) error {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("distill: create dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("distill: create file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, ex := range dp.Examples {
		record := map[string]interface{}{
			"messages": buildMessages(ex),
		}
		if err := enc.Encode(record); err != nil {
			return fmt.Errorf("distill: encode openai: %w", err)
		}
	}

	return nil
}

// ExportAnthropicFormat exports examples in Anthropic fine-tuning format.
// Format: {"system":"...","messages":[{"role":"user","content":"..."},{"role":"assistant","content":"..."}]}
func (dp *DistillationPipeline) ExportAnthropicFormat(path string) error {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("distill: create dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("distill: create file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, ex := range dp.Examples {
		messages := []map[string]string{
			{"role": "user", "content": ex.UserMessage},
			{"role": "assistant", "content": ex.AssistantResponse},
		}
		record := map[string]interface{}{
			"system":   ex.SystemPrompt,
			"messages": messages,
		}
		if err := enc.Encode(record); err != nil {
			return fmt.Errorf("distill: encode anthropic: %w", err)
		}
	}

	return nil
}

// Filter returns examples matching the given minimum quality and any of the specified tags.
// If tags is empty, only quality filtering is applied.
func (dp *DistillationPipeline) Filter(minQuality float64, tags []string) []DistillExample {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	var results []DistillExample
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}

	for _, ex := range dp.Examples {
		if ex.Quality < minQuality {
			continue
		}
		if len(tags) > 0 {
			matched := false
			for _, t := range ex.Tags {
				if tagSet[t] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		results = append(results, ex)
	}

	return results
}

// Deduplicate removes near-duplicate examples (>0.9 similarity based on content hashing and overlap).
func (dp *DistillationPipeline) Deduplicate() {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if len(dp.Examples) <= 1 {
		return
	}

	kept := make([]DistillExample, 0, len(dp.Examples))
	seen := make([]string, 0, len(dp.Examples))

	for _, ex := range dp.Examples {
		content := ex.UserMessage + ex.AssistantResponse
		isDuplicate := false

		for _, prev := range seen {
			if similarity(content, prev) > 0.9 {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			kept = append(kept, ex)
			seen = append(seen, content)
		}
	}

	dp.Examples = kept
}

// Stats computes aggregate statistics about the distillation pipeline.
func (dp *DistillationPipeline) Stats() DistillStats {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	stats := DistillStats{
		TotalExamples: len(dp.Examples),
		ByModel:       make(map[string]int),
		ByTag:         make(map[string]int),
	}

	if len(dp.Examples) == 0 {
		return stats
	}

	var totalQuality float64
	for _, ex := range dp.Examples {
		totalQuality += ex.Quality
		stats.TotalTokens += ex.Tokens
		stats.ByModel[ex.Model]++
		for _, tag := range ex.Tags {
			stats.ByTag[tag]++
		}
	}

	stats.AvgQuality = totalQuality / float64(len(dp.Examples))
	// Estimate cost at ~$0.008 per 1K tokens for fine-tuning
	stats.EstimatedCost = float64(stats.TotalTokens) / 1000.0 * 0.008

	return stats
}

// FormatStats returns a human-readable summary of the distillation pipeline.
func (dp *DistillationPipeline) FormatStats() string {
	stats := dp.Stats()

	var sb strings.Builder
	sb.WriteString("Distillation Pipeline:\n")
	sb.WriteString("─────────────────────\n")
	sb.WriteString(fmt.Sprintf("Examples: %d\n", stats.TotalExamples))
	sb.WriteString(fmt.Sprintf("Avg quality: %.2f\n", stats.AvgQuality))

	// Source model breakdown
	if len(stats.ByModel) > 0 {
		sb.WriteString("Source: ")
		modelParts := formatModelBreakdown(stats.ByModel, stats.TotalExamples)
		sb.WriteString(strings.Join(modelParts, ", "))
		sb.WriteString("\n")
	}

	// Tag breakdown
	if len(stats.ByTag) > 0 {
		sb.WriteString("Tags: ")
		tagParts := formatTagBreakdown(stats.ByTag, stats.TotalExamples)
		sb.WriteString(strings.Join(tagParts, ", "))
		sb.WriteString("\n")
	}

	// Token and cost info
	tokenStr := formatDistillTokenCount(stats.TotalTokens)
	sb.WriteString(fmt.Sprintf("Tokens: %s (est. fine-tuning cost: $%.2f)\n", tokenStr, stats.EstimatedCost))

	// Target readiness
	dp.mu.RLock()
	target := dp.TargetModel
	dp.mu.RUnlock()
	sb.WriteString(fmt.Sprintf("Ready for: %s fine-tuning\n", target))

	return sb.String()
}

// Save persists the pipeline state to disk.
func (dp *DistillationPipeline) Save() error {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	if err := os.MkdirAll(dp.Dir, 0o755); err != nil {
		return fmt.Errorf("distill: create dir: %w", err)
	}

	path := filepath.Join(dp.Dir, "distill_pipeline.json")
	data, err := json.MarshalIndent(dp, "", "  ")
	if err != nil {
		return fmt.Errorf("distill: marshal: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

// Load restores the pipeline state from disk.
func (dp *DistillationPipeline) Load() error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	path := filepath.Join(dp.Dir, "distill_pipeline.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("distill: read: %w", err)
	}

	var loaded DistillationPipeline
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("distill: unmarshal: %w", err)
	}

	dp.Examples = loaded.Examples
	dp.TargetModel = loaded.TargetModel
	dp.SourceModel = loaded.SourceModel
	dp.MinQuality = loaded.MinQuality

	return nil
}

// Prune keeps only the top N examples by quality, discarding the rest.
func (dp *DistillationPipeline) Prune(maxExamples int) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if len(dp.Examples) <= maxExamples {
		return
	}

	sort.Slice(dp.Examples, func(i, j int) bool {
		return dp.Examples[i].Quality > dp.Examples[j].Quality
	})

	dp.Examples = dp.Examples[:maxExamples]
}

// --- Helper functions ---

func generateDistillID(system, user, assistant string) string {
	h := sha256.New()
	h.Write([]byte(system))
	h.Write([]byte(user))
	h.Write([]byte(assistant))
	return fmt.Sprintf("distill_%x", h.Sum(nil)[:8])
}

func estimateDistillTokens(text string) int {
	// Rough estimate: ~4 characters per token
	return len(text) / 4
}

func inferTags(user, assistant string) []string {
	var tags []string
	combined := strings.ToLower(user + " " + assistant)

	tagKeywords := map[string][]string{
		"coding":  {"func ", "function", "implement", "code", "package", "import", "class"},
		"review":  {"review", "feedback", "suggest", "improve", "refactor"},
		"debug":   {"error", "bug", "fix", "debug", "stack trace", "panic"},
		"test":    {"test", "assert", "expect", "mock", "coverage"},
		"docs":    {"document", "readme", "explain", "comment", "description"},
		"deploy":  {"deploy", "ci/cd", "pipeline", "docker", "kubernetes"},
		"design":  {"architecture", "design", "pattern", "interface", "struct"},
		"config":  {"config", "setting", "environment", "variable", "yaml", "json"},
	}

	for tag, keywords := range tagKeywords {
		for _, kw := range keywords {
			if strings.Contains(combined, kw) {
				tags = append(tags, tag)
				break
			}
		}
	}

	if len(tags) == 0 {
		tags = append(tags, "general")
	}

	sort.Strings(tags)
	return tags
}

func buildMessagesRecord(ex DistillExample) map[string]interface{} {
	return map[string]interface{}{
		"messages": buildMessages(ex),
	}
}

func buildMessages(ex DistillExample) []map[string]string {
	messages := make([]map[string]string, 0, 3)

	if ex.SystemPrompt != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": ex.SystemPrompt,
		})
	}

	messages = append(messages, map[string]string{
		"role":    "user",
		"content": ex.UserMessage,
	})

	messages = append(messages, map[string]string{
		"role":    "assistant",
		"content": ex.AssistantResponse,
	})

	return messages
}

// similarity computes a rough similarity score between two strings using trigram overlap.
func similarity(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}

	trigramsA := trigrams(a)
	trigramsB := trigrams(b)

	if len(trigramsA) == 0 || len(trigramsB) == 0 {
		return 0.0
	}

	intersection := 0
	for tri := range trigramsA {
		if trigramsB[tri] {
			intersection++
		}
	}

	union := len(trigramsA) + len(trigramsB) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

func trigrams(s string) map[string]bool {
	result := make(map[string]bool)
	if len(s) < 3 {
		result[s] = true
		return result
	}
	for i := 0; i <= len(s)-3; i++ {
		result[s[i:i+3]] = true
	}
	return result
}

func formatModelBreakdown(byModel map[string]int, total int) []string {
	type modelCount struct {
		model string
		count int
	}

	models := make([]modelCount, 0, len(byModel))
	for m, c := range byModel {
		models = append(models, modelCount{m, c})
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].count > models[j].count
	})

	parts := make([]string, 0, len(models))
	for _, mc := range models {
		pct := int(math.Round(float64(mc.count) / float64(total) * 100))
		parts = append(parts, fmt.Sprintf("%s (%d%%)", mc.model, pct))
	}
	return parts
}

func formatTagBreakdown(byTag map[string]int, total int) []string {
	type tagCount struct {
		tag   string
		count int
	}

	tags := make([]tagCount, 0, len(byTag))
	for t, c := range byTag {
		tags = append(tags, tagCount{t, c})
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].count > tags[j].count
	})

	parts := make([]string, 0, len(tags))
	for _, tc := range tags {
		pct := int(math.Round(float64(tc.count) / float64(total) * 100))
		parts = append(parts, fmt.Sprintf("%s (%d%%)", tc.tag, pct))
	}
	return parts
}

func formatDistillTokenCount(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
