package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Insight represents a learned insight from a previous session.
type Insight struct {
	ID           string    `json:"id"`
	Content      string    `json:"content"`
	Category     string    `json:"category"` // "approach", "tool_usage", "avoidance", "preference"
	Language     string    `json:"language"`
	Confidence   float64   `json:"confidence"`
	SuccessCount int       `json:"success_count"`
	CreatedAt    time.Time `json:"created_at"`
	LastUsed     time.Time `json:"last_used"`
}

// SessionConvention represents a project convention discovered during a session.
type SessionConvention struct {
	ID           string   `json:"id"`
	Rule         string   `json:"rule"`
	Examples     []string `json:"examples"`
	Source       string   `json:"source"`
	Confidence   float64  `json:"confidence"`
	AppliedCount int      `json:"applied_count"`
}

// FailurePattern represents a recorded failure and its resolution.
type FailurePattern struct {
	ID          string    `json:"id"`
	Pattern     string    `json:"pattern"`
	Context     string    `json:"context"`
	Resolution  string    `json:"resolution"`
	Language    string    `json:"language"`
	Occurrences int       `json:"occurrences"`
	LastSeen    time.Time `json:"last_seen"`
}

// LearnerStats holds aggregate statistics about the cross-session learner.
type LearnerStats struct {
	InsightCount    int     `json:"insight_count"`
	ConventionCount int     `json:"convention_count"`
	FailureCount    int     `json:"failure_count"`
	AvgConfidence   float64 `json:"avg_confidence"`
}

// CrossSessionLearner transfers insights, conventions, and patterns across sessions.
type CrossSessionLearner struct {
	Insights        []Insight           `json:"insights"`
	Conventions     []SessionConvention `json:"conventions"`
	FailurePatterns []FailurePattern    `json:"failure_patterns"`
	Dir             string              `json:"-"`
	mu              sync.RWMutex
}

// NewCrossSessionLearner creates a new CrossSessionLearner that persists data in dir.
func NewCrossSessionLearner(dir string) *CrossSessionLearner {
	return &CrossSessionLearner{
		Insights:        []Insight{},
		Conventions:     []SessionConvention{},
		FailurePatterns: []FailurePattern{},
		Dir:             dir,
	}
}

// LearnFromOutcome records the result of a task attempt. If successful, it extracts
// an insight about what worked. If failed, it records a failure pattern.
func (c *CrossSessionLearner) LearnFromOutcome(task, approach string, success bool, toolsUsed []string, filesModified []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	if success {
		// Extract language from file extensions
		lang := detectLanguageFromFiles(filesModified)

		// Determine category based on tools and approach
		category := categorizeApproach(approach, toolsUsed)

		content := fmt.Sprintf("For task '%s': %s", task, approach)
		if len(toolsUsed) > 0 {
			content += fmt.Sprintf(" (tools: %s)", strings.Join(toolsUsed, ", "))
		}

		id := fmt.Sprintf("insight-%d", now.UnixNano())

		// Check if a similar insight already exists
		for i, ins := range c.Insights {
			if ins.Category == category && containsOverlap(ins.Content, task) {
				c.Insights[i].SuccessCount++
				c.Insights[i].LastUsed = now
				c.Insights[i].Confidence = clampConfidence(ins.Confidence + 0.05)
				return
			}
		}

		c.Insights = append(c.Insights, Insight{
			ID:           id,
			Content:      content,
			Category:     category,
			Language:     lang,
			Confidence:   0.6,
			SuccessCount: 1,
			CreatedAt:    now,
			LastUsed:     now,
		})
	} else {
		lang := detectLanguageFromFiles(filesModified)
		id := fmt.Sprintf("failure-%d", now.UnixNano())

		pattern := fmt.Sprintf("Failed: %s with approach: %s", task, approach)

		c.FailurePatterns = append(c.FailurePatterns, FailurePattern{
			ID:          id,
			Pattern:     pattern,
			Context:     task,
			Resolution:  "",
			Language:    lang,
			Occurrences: 1,
			LastSeen:    now,
		})
	}
}

// LearnConvention records a project convention discovered during a session.
func (c *CrossSessionLearner) LearnConvention(rule string, examples []string, source string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	id := fmt.Sprintf("conv-%d", now.UnixNano())

	// Check if this convention already exists
	for i, conv := range c.Conventions {
		if conv.Rule == rule {
			c.Conventions[i].AppliedCount++
			c.Conventions[i].Confidence = clampConfidence(conv.Confidence + 0.05)
			// Merge new examples
			for _, ex := range examples {
				if !containsString(c.Conventions[i].Examples, ex) {
					c.Conventions[i].Examples = append(c.Conventions[i].Examples, ex)
				}
			}
			return
		}
	}

	c.Conventions = append(c.Conventions, SessionConvention{
		ID:           id,
		Rule:         rule,
		Examples:     examples,
		Source:       source,
		Confidence:   0.7,
		AppliedCount: 1,
	})
}

// RecordFailure records a failure pattern and how it was resolved.
func (c *CrossSessionLearner) RecordFailure(pattern, context, resolution string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Check if this failure pattern already exists
	for i, fp := range c.FailurePatterns {
		if fp.Pattern == pattern {
			c.FailurePatterns[i].Occurrences++
			c.FailurePatterns[i].LastSeen = now
			if resolution != "" {
				c.FailurePatterns[i].Resolution = resolution
			}
			return
		}
	}

	id := fmt.Sprintf("fail-%d", now.UnixNano())
	c.FailurePatterns = append(c.FailurePatterns, FailurePattern{
		ID:          id,
		Pattern:     pattern,
		Context:     context,
		Resolution:  resolution,
		Language:    "",
		Occurrences: 1,
		LastSeen:    now,
	})
}

// GetRelevantInsights returns insights relevant to the current task, scored by
// keyword overlap, confidence, and recency.
func (c *CrossSessionLearner) GetRelevantInsights(task string, limit int) []Insight {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.Insights) == 0 {
		return nil
	}

	type scored struct {
		insight Insight
		score   float64
	}

	taskWords := tokenize(task)
	now := time.Now()

	var results []scored
	for _, ins := range c.Insights {
		// Keyword overlap score
		insightWords := tokenize(ins.Content)
		overlap := wordOverlap(taskWords, insightWords)

		// Confidence score
		confScore := ins.Confidence

		// Recency score: decay over 30 days
		daysSinceUse := now.Sub(ins.LastUsed).Hours() / 24
		recencyScore := 1.0
		if daysSinceUse > 0 {
			recencyScore = 1.0 / (1.0 + daysSinceUse/30.0)
		}

		totalScore := overlap*0.5 + confScore*0.3 + recencyScore*0.2

		if totalScore > 0.1 {
			results = append(results, scored{insight: ins, score: totalScore})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if limit <= 0 {
		limit = 10
	}
	if limit > len(results) {
		limit = len(results)
	}

	out := make([]Insight, limit)
	for i := 0; i < limit; i++ {
		out[i] = results[i].insight
	}
	return out
}

// GetConventions returns all learned conventions.
func (c *CrossSessionLearner) GetConventions() []SessionConvention {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]SessionConvention, len(c.Conventions))
	copy(result, c.Conventions)
	return result
}

// GetFailureResolutions finds past failures matching an error message and returns
// them with their resolutions.
func (c *CrossSessionLearner) GetFailureResolutions(errorMsg string) []FailurePattern {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.FailurePatterns) == 0 {
		return nil
	}

	errorWords := tokenize(errorMsg)
	var matches []FailurePattern

	for _, fp := range c.FailurePatterns {
		patternWords := tokenize(fp.Pattern)
		contextWords := tokenize(fp.Context)

		overlapPattern := wordOverlap(errorWords, patternWords)
		overlapContext := wordOverlap(errorWords, contextWords)

		if overlapPattern > 0.2 || overlapContext > 0.2 {
			matches = append(matches, fp)
		}
	}

	// Sort by occurrences descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Occurrences > matches[j].Occurrences
	})

	return matches
}

// BuildSessionPrimer formats all relevant learning for injection into a new session.
func (c *CrossSessionLearner) BuildSessionPrimer(task string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("## Cross-Session Learning\n")

	// Relevant insights
	insights := c.getRelevantInsightsUnlocked(task, 5)
	if len(insights) > 0 {
		sb.WriteString("\n### Relevant Insights\n")
		for _, ins := range insights {
			sb.WriteString(fmt.Sprintf("- %s (confidence: %.2g)\n", ins.Content, ins.Confidence))
		}
	}

	// Conventions
	if len(c.Conventions) > 0 {
		sb.WriteString("\n### Conventions\n")
		for _, conv := range c.Conventions {
			sb.WriteString(fmt.Sprintf("- %s\n", conv.Rule))
		}
	}

	// Known pitfalls (failures with resolutions)
	pitfalls := c.getPitfallsUnlocked()
	if len(pitfalls) > 0 {
		sb.WriteString("\n### Known Pitfalls\n")
		for _, fp := range pitfalls {
			if fp.Resolution != "" {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", fp.Pattern, fp.Resolution))
			}
		}
	}

	return sb.String()
}

// Decay reduces confidence of old unused insights by the given factor.
func (c *CrossSessionLearner) Decay(factor float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Insights {
		c.Insights[i].Confidence *= factor
		if c.Insights[i].Confidence < 0.01 {
			c.Insights[i].Confidence = 0.01
		}
	}

	for i := range c.Conventions {
		c.Conventions[i].Confidence *= factor
		if c.Conventions[i].Confidence < 0.01 {
			c.Conventions[i].Confidence = 0.01
		}
	}
}

// Save persists the learner state to disk.
func (c *CrossSessionLearner) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return fmt.Errorf("create learner dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal learner: %w", err)
	}

	path := filepath.Join(c.Dir, "cross_session.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write learner file: %w", err)
	}

	return nil
}

// Load restores the learner state from disk.
func (c *CrossSessionLearner) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	path := filepath.Join(c.Dir, "cross_session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read learner file: %w", err)
	}

	var loaded CrossSessionLearner
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("unmarshal learner: %w", err)
	}

	c.Insights = loaded.Insights
	c.Conventions = loaded.Conventions
	c.FailurePatterns = loaded.FailurePatterns
	return nil
}

// Stats returns aggregate statistics about the learner.
func (c *CrossSessionLearner) Stats() LearnerStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := LearnerStats{
		InsightCount:    len(c.Insights),
		ConventionCount: len(c.Conventions),
		FailureCount:    len(c.FailurePatterns),
	}

	if len(c.Insights) > 0 {
		var total float64
		for _, ins := range c.Insights {
			total += ins.Confidence
		}
		stats.AvgConfidence = total / float64(len(c.Insights))
	}

	return stats
}

// getRelevantInsightsUnlocked is the internal version without locking.
func (c *CrossSessionLearner) getRelevantInsightsUnlocked(task string, limit int) []Insight {
	if len(c.Insights) == 0 {
		return nil
	}

	type scored struct {
		insight Insight
		score   float64
	}

	taskWords := tokenize(task)
	now := time.Now()

	var results []scored
	for _, ins := range c.Insights {
		insightWords := tokenize(ins.Content)
		overlap := wordOverlap(taskWords, insightWords)
		confScore := ins.Confidence

		daysSinceUse := now.Sub(ins.LastUsed).Hours() / 24
		recencyScore := 1.0
		if daysSinceUse > 0 {
			recencyScore = 1.0 / (1.0 + daysSinceUse/30.0)
		}

		totalScore := overlap*0.5 + confScore*0.3 + recencyScore*0.2
		if totalScore > 0.05 {
			results = append(results, scored{insight: ins, score: totalScore})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if limit > len(results) {
		limit = len(results)
	}

	out := make([]Insight, limit)
	for i := 0; i < limit; i++ {
		out[i] = results[i].insight
	}
	return out
}

// getPitfallsUnlocked returns failure patterns that have resolutions.
func (c *CrossSessionLearner) getPitfallsUnlocked() []FailurePattern {
	var pitfalls []FailurePattern
	for _, fp := range c.FailurePatterns {
		if fp.Resolution != "" {
			pitfalls = append(pitfalls, fp)
		}
	}
	return pitfalls
}

// --- Helper functions ---

func detectLanguageFromFiles(files []string) string {
	extCount := make(map[string]int)
	for _, f := range files {
		ext := strings.TrimPrefix(filepath.Ext(f), ".")
		switch ext {
		case "go":
			extCount["go"]++
		case "py":
			extCount["python"]++
		case "js", "jsx":
			extCount["javascript"]++
		case "ts", "tsx":
			extCount["typescript"]++
		case "rs":
			extCount["rust"]++
		case "rb":
			extCount["ruby"]++
		case "java":
			extCount["java"]++
		default:
			extCount["generic"]++
		}
	}

	best := "generic"
	bestCount := 0
	for lang, count := range extCount {
		if count > bestCount && lang != "generic" {
			best = lang
			bestCount = count
		}
	}
	return best
}

func categorizeApproach(approach string, toolsUsed []string) string {
	lower := strings.ToLower(approach)

	if strings.Contains(lower, "avoid") || strings.Contains(lower, "don't") || strings.Contains(lower, "skip") {
		return "avoidance"
	}
	if strings.Contains(lower, "prefer") || strings.Contains(lower, "always") || strings.Contains(lower, "use") {
		return "preference"
	}
	if len(toolsUsed) > 0 {
		return "tool_usage"
	}
	return "approach"
}

func tokenize(s string) []string {
	lower := strings.ToLower(s)
	// Split on non-alphanumeric characters
	var words []string
	var current strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current.WriteRune(r)
		} else {
			if current.Len() > 2 { // Only keep words longer than 2 chars
				words = append(words, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() > 2 {
		words = append(words, current.String())
	}
	return words
}

func wordOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	setB := make(map[string]bool, len(b))
	for _, w := range b {
		setB[w] = true
	}

	matches := 0
	for _, w := range a {
		if setB[w] {
			matches++
		}
	}

	return float64(matches) / float64(len(a))
}

func containsOverlap(content, task string) bool {
	contentWords := tokenize(content)
	taskWords := tokenize(task)
	return wordOverlap(taskWords, contentWords) > 0.5
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func clampConfidence(c float64) float64 {
	if c > 1.0 {
		return 1.0
	}
	if c < 0.0 {
		return 0.0
	}
	return c
}
