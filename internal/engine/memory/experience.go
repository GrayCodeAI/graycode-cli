package memory

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

// Experience represents a single recorded task completion with its context and outcome.
type Experience struct {
	ID            string        `json:"id"`
	Task          string        `json:"task"`
	Approach      string        `json:"approach"`
	Steps         []string      `json:"steps"`
	Outcome       string        `json:"outcome"`
	ToolsUsed     []string      `json:"tools_used"`
	FilesModified []string      `json:"files_modified"`
	TokensUsed    int           `json:"tokens_used"`
	Duration      time.Duration `json:"duration"`
	Score         float64       `json:"score"`
	Tags          []string      `json:"tags"`
	CreatedAt     time.Time     `json:"created_at"`
	UsedCount     int           `json:"used_count"`
}

// ExperienceStats holds aggregate statistics about the experience store.
type ExperienceStats struct {
	TotalExperiences int            `json:"total_experiences"`
	AvgScore         float64        `json:"avg_score"`
	AvgTokens        int            `json:"avg_tokens"`
	AvgDuration      time.Duration  `json:"avg_duration"`
	ByOutcome        map[string]int `json:"by_outcome"`
	TopTags          []string       `json:"top_tags"`
	TopTools         []string       `json:"top_tools"`
}

// ExperienceStore manages a collection of experiences with persistence.
type ExperienceStore struct {
	Experiences []*Experience
	Dir         string
	mu          sync.RWMutex
}

// NewExperienceStore creates a new ExperienceStore that persists to the given directory.
func NewExperienceStore(dir string) *ExperienceStore {
	return &ExperienceStore{
		Experiences: make([]*Experience, 0),
		Dir:         dir,
	}
}

// Record creates a new experience entry from a completed task.
func (es *ExperienceStore) Record(task, approach, outcome string, steps, tools, files []string, tokens int, duration time.Duration) *Experience {
	es.mu.Lock()
	defer es.mu.Unlock()

	exp := &Experience{
		ID:            generateExperienceID(task, time.Now()),
		Task:          task,
		Approach:      approach,
		Steps:         steps,
		Outcome:       outcome,
		ToolsUsed:     tools,
		FilesModified: files,
		TokensUsed:    tokens,
		Duration:      duration,
		Score:         scoreOutcome(outcome),
		Tags:          extractTags(task),
		CreatedAt:     time.Now(),
		UsedCount:     0,
	}

	es.Experiences = append(es.Experiences, exp)
	return exp
}

// FindRelevant returns the most relevant experiences for a given task description.
func (es *ExperienceStore) FindRelevant(task string, limit int) []*Experience {
	es.mu.RLock()
	defer es.mu.RUnlock()

	if len(es.Experiences) == 0 || limit <= 0 {
		return nil
	}

	type scored struct {
		exp   *Experience
		score float64
	}

	taskKeywords := expTokenize(task)
	now := time.Now()

	var results []scored
	for _, exp := range es.Experiences {
		expKeywords := expTokenize(exp.Task + " " + exp.Approach)

		// Keyword overlap score
		overlap := expJaccardSimilarity(taskKeywords, expKeywords)
		if overlap == 0 {
			continue
		}

		// Boost by success score
		successBoost := exp.Score

		// Boost by recency (decay over 30 days)
		age := now.Sub(exp.CreatedAt)
		recencyBoost := math.Exp(-age.Hours() / (30 * 24))

		// Boost by usage (diminishing returns)
		usageBoost := math.Log2(float64(exp.UsedCount+1)) / 10.0

		finalScore := overlap*(0.5+successBoost*0.3) + recencyBoost*0.15 + usageBoost*0.05

		results = append(results, scored{exp: exp, score: finalScore})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if limit > len(results) {
		limit = len(results)
	}

	out := make([]*Experience, limit)
	for i := 0; i < limit; i++ {
		out[i] = results[i].exp
	}
	return out
}

// BuildExperienceContext formats relevant experiences for prompt injection.
func (es *ExperienceStore) BuildExperienceContext(task string, maxTokens int) string {
	relevant := es.FindRelevant(task, 5)
	if len(relevant) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Relevant Past Experiences\n\n")

	estimatedTokens := 10 // header tokens
	for _, exp := range relevant {
		entry := formatExperienceEntry(exp)
		entryTokens := expEstimateTokens(entry)

		if estimatedTokens+entryTokens > maxTokens {
			break
		}

		b.WriteString(entry)
		b.WriteString("\n")
		estimatedTokens += entryTokens

		// Increment usage count
		es.mu.Lock()
		exp.UsedCount++
		es.mu.Unlock()
	}

	return b.String()
}

// Generalize creates a generalized version of an experience by stripping specific details.
func (es *ExperienceStore) Generalize(exp *Experience) *Experience {
	generalized := &Experience{
		ID:         exp.ID + "-generalized",
		Task:       exp.Task,
		Approach:   exp.Approach,
		Steps:      make([]string, len(exp.Steps)),
		Outcome:    exp.Outcome,
		ToolsUsed:  exp.ToolsUsed,
		TokensUsed: exp.TokensUsed,
		Duration:   exp.Duration,
		Score:      exp.Score,
		Tags:       exp.Tags,
		CreatedAt:  exp.CreatedAt,
		UsedCount:  exp.UsedCount,
	}

	// Generalize file paths to patterns
	generalizedFiles := make([]string, 0, len(exp.FilesModified))
	for _, f := range exp.FilesModified {
		generalizedFiles = append(generalizedFiles, generalizePath(f))
	}
	generalized.FilesModified = generalizedFiles

	// Generalize steps
	for i, step := range exp.Steps {
		generalized.Steps[i] = generalizeText(step)
	}

	// Generalize approach
	generalized.Approach = generalizeText(exp.Approach)

	return generalized
}

// Deduplicate removes experiences that are too similar to each other,
// keeping the one with the higher score.
func (es *ExperienceStore) Deduplicate() int {
	es.mu.Lock()
	defer es.mu.Unlock()

	if len(es.Experiences) <= 1 {
		return 0
	}

	removed := 0
	keep := make(map[int]bool)
	for i := range es.Experiences {
		keep[i] = true
	}

	for i := 0; i < len(es.Experiences); i++ {
		if !keep[i] {
			continue
		}
		for j := i + 1; j < len(es.Experiences); j++ {
			if !keep[j] {
				continue
			}
			textI := es.Experiences[i].Task + " " + es.Experiences[i].Approach
			textJ := es.Experiences[j].Task + " " + es.Experiences[j].Approach
			sim := expJaccardSimilarity(expTokenize(textI), expTokenize(textJ))
			if sim > 0.8 {
				// Remove the one with the lower score
				if es.Experiences[i].Score >= es.Experiences[j].Score {
					keep[j] = false
				} else {
					keep[i] = false
					break
				}
				removed++
			}
		}
	}

	filtered := make([]*Experience, 0, len(es.Experiences)-removed)
	for i, exp := range es.Experiences {
		if keep[i] {
			filtered = append(filtered, exp)
		}
	}
	es.Experiences = filtered
	return removed
}

// Prune removes experiences older than maxAge or below minScore.
func (es *ExperienceStore) Prune(maxAge time.Duration, minScore float64) int {
	es.mu.Lock()
	defer es.mu.Unlock()

	now := time.Now()
	pruned := 0
	filtered := make([]*Experience, 0, len(es.Experiences))

	for _, exp := range es.Experiences {
		age := now.Sub(exp.CreatedAt)
		if age > maxAge || exp.Score < minScore {
			pruned++
			continue
		}
		filtered = append(filtered, exp)
	}

	es.Experiences = filtered
	return pruned
}

// Save persists all experiences to a JSON file in the store directory.
func (es *ExperienceStore) Save() error {
	es.mu.RLock()
	defer es.mu.RUnlock()

	if es.Dir == "" {
		return fmt.Errorf("experience store: no directory configured")
	}

	if err := os.MkdirAll(es.Dir, 0o750); err != nil {
		return fmt.Errorf("experience store: create dir: %w", err)
	}

	data, err := json.MarshalIndent(es.Experiences, "", "  ")
	if err != nil {
		return fmt.Errorf("experience store: marshal: %w", err)
	}

	path := filepath.Join(es.Dir, "experiences.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("experience store: write: %w", err)
	}

	return nil
}

// Load reads experiences from the JSON file in the store directory.
func (es *ExperienceStore) Load() error {
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.Dir == "" {
		return fmt.Errorf("experience store: no directory configured")
	}

	path := filepath.Join(es.Dir, "experiences.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no file yet is fine
		}
		return fmt.Errorf("experience store: read: %w", err)
	}

	var exps []*Experience
	if err := json.Unmarshal(data, &exps); err != nil {
		return fmt.Errorf("experience store: unmarshal: %w", err)
	}

	es.Experiences = exps
	return nil
}

// Stats returns aggregate statistics about the experience store.
func (es *ExperienceStore) Stats() ExperienceStats {
	es.mu.RLock()
	defer es.mu.RUnlock()

	stats := ExperienceStats{
		TotalExperiences: len(es.Experiences),
		ByOutcome:        make(map[string]int),
	}

	if len(es.Experiences) == 0 {
		return stats
	}

	var totalScore float64
	var totalTokens int
	var totalDuration time.Duration
	tagCounts := make(map[string]int)
	toolCounts := make(map[string]int)

	for _, exp := range es.Experiences {
		totalScore += exp.Score
		totalTokens += exp.TokensUsed
		totalDuration += exp.Duration
		stats.ByOutcome[exp.Outcome]++

		for _, tag := range exp.Tags {
			tagCounts[tag]++
		}
		for _, tool := range exp.ToolsUsed {
			toolCounts[tool]++
		}
	}

	n := len(es.Experiences)
	stats.AvgScore = totalScore / float64(n)
	stats.AvgTokens = totalTokens / n
	stats.AvgDuration = totalDuration / time.Duration(n)
	stats.TopTags = topN(tagCounts, 10)
	stats.TopTools = topN(toolCounts, 10)

	return stats
}

// --- Helper functions ---

func generateExperienceID(task string, t time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", task, t.UnixNano())))
	return fmt.Sprintf("exp-%x", h[:8])
}

func scoreOutcome(outcome string) float64 {
	lower := strings.ToLower(outcome)
	switch {
	case strings.Contains(lower, "partial"):
		return 0.5
	case strings.Contains(lower, "failure"), strings.Contains(lower, "failed"):
		return 0.0
	case strings.Contains(lower, "success"):
		return 1.0
	default:
		return 0.5
	}
}

func extractTags(task string) []string {
	// Common programming keywords to extract as tags
	keywords := []string{
		"api", "auth", "bug", "build", "cache", "ci", "cli", "config",
		"database", "db", "debug", "deploy", "docker", "docs", "error",
		"feature", "fix", "frontend", "grpc", "handler", "http", "implement",
		"jwt", "kubernetes", "lint", "logging", "middleware", "migrate",
		"monitor", "mutex", "optimize", "performance", "queue", "race",
		"refactor", "rest", "route", "security", "server", "sql", "test",
		"timeout", "ui", "validate", "websocket", "worker",
	}

	lower := strings.ToLower(task)

	var tags []string
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			tags = append(tags, kw)
		}
	}

	return tags
}

func expTokenize(text string) []string {
	lower := strings.ToLower(text)
	// Split on non-alphanumeric characters
	var words []string
	var current strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	// Remove very short words (stop words approximation)
	var filtered []string
	for _, w := range words {
		if len(w) > 2 {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

func expJaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}

	setA := make(map[string]bool, len(a))
	for _, w := range a {
		setA[w] = true
	}

	setB := make(map[string]bool, len(b))
	for _, w := range b {
		setB[w] = true
	}

	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}

func formatExperienceEntry(exp *Experience) string {
	var b strings.Builder

	// Format relative time
	age := time.Since(exp.CreatedAt)
	ageStr := formatAge(age)

	b.WriteString(fmt.Sprintf("Task: %q (%s, %s)\n", exp.Task, exp.Outcome, ageStr))
	b.WriteString(fmt.Sprintf("Approach: %s\n", exp.Approach))

	if len(exp.ToolsUsed) > 0 {
		b.WriteString(fmt.Sprintf("Tools: %s\n", strings.Join(exp.ToolsUsed, ", ")))
	}

	// Extract key insight from the last step if available
	if len(exp.Steps) > 0 {
		lastStep := exp.Steps[len(exp.Steps)-1]
		b.WriteString(fmt.Sprintf("Key insight: %s\n", lastStep))
	}

	return b.String()
}

func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

func expEstimateTokens(text string) int {
	// Rough estimate: ~4 characters per token
	return len(text) / 4
}

func generalizePath(path string) string {
	// Replace specific filenames with patterns based on extension
	ext := filepath.Ext(path)
	dir := filepath.Dir(path)

	// Generalize directory to pattern
	parts := strings.Split(dir, string(filepath.Separator))
	var generalizedParts []string
	for _, p := range parts {
		if p == "" {
			continue
		}
		// Keep structural directories, generalize specific ones
		switch p {
		case "src", "lib", "pkg", "cmd", "internal", "test", "tests",
			"api", "config", "docs", "scripts", "tools", "vendor":
			generalizedParts = append(generalizedParts, p)
		default:
			generalizedParts = append(generalizedParts, "<module>")
		}
	}

	if ext != "" {
		return strings.Join(generalizedParts, "/") + "/*" + ext
	}
	return strings.Join(generalizedParts, "/") + "/<file>"
}

func generalizeText(text string) string {
	// Replace file paths with generic patterns
	words := strings.Fields(text)
	var result []string
	for _, w := range words {
		if strings.Contains(w, "/") && (strings.Contains(w, ".go") ||
			strings.Contains(w, ".py") || strings.Contains(w, ".ts") ||
			strings.Contains(w, ".js") || strings.Contains(w, ".rs")) {
			ext := filepath.Ext(w)
			result = append(result, "<path>"+ext)
		} else {
			result = append(result, w)
		}
	}
	return strings.Join(result, " ")
}

func topN(counts map[string]int, n int) []string {
	type kv struct {
		key   string
		count int
	}

	var items []kv
	for k, v := range counts {
		items = append(items, kv{key: k, count: v})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].count > items[j].count
	})

	if n > len(items) {
		n = len(items)
	}

	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = items[i].key
	}
	return result
}
