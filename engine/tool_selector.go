package engine

import (
	"fmt"
	"strings"
	"sync"
)

// ToolInfo describes a single tool available to the LLM.
type ToolInfo struct {
	Name     string
	Category string // "file", "search", "exec", "web", "agent", "system"
	Cost     string // "free", "cheap", "expensive"
	ReadOnly bool
}

// ToolSelection holds the result of selecting tools for a task.
type ToolSelection struct {
	Recommended []string
	Excluded    []string
	Reason      string
	Confidence  float64
}

// ToolSelector recommends which tools to send to the LLM based on the task.
type ToolSelector struct {
	AllTools     []ToolInfo
	UsageHistory map[string]int
	TaskPatterns map[string][]string
	mu           sync.RWMutex
}

// TaskToolMap provides built-in mappings from task intents to tool names.
var TaskToolMap = map[string][]string{
	"read code": {"Read", "Grep", "Glob", "LS"},
	"write code": {"Read", "Edit", "Write", "Bash"},
	"debug":      {"Read", "Grep", "Bash", "Edit"},
	"search":     {"Grep", "Glob", "WebSearch"},
	"test":       {"Bash", "Read", "Write"},
	"review":     {"Read", "Grep", "Glob"},
	"refactor":   {"Read", "Edit", "Write", "Grep", "Bash"},
	"deploy":     {"Bash", "Read", "Write"},
}

// taskKeywords maps keywords found in a task description to intent categories.
var taskKeywords = map[string][]string{
	"read":       {"read code"},
	"understand": {"read code"},
	"explore":    {"read code"},
	"write":      {"write code"},
	"implement":  {"write code"},
	"create":     {"write code"},
	"add":        {"write code"},
	"debug":      {"debug"},
	"fix":        {"debug"},
	"bug":        {"debug"},
	"error":      {"debug"},
	"issue":      {"debug"},
	"search":     {"search"},
	"find":       {"search"},
	"look":       {"search"},
	"test":       {"test"},
	"verify":     {"test"},
	"check":      {"test"},
	"review":     {"review"},
	"inspect":    {"review"},
	"audit":      {"review"},
	"refactor":   {"refactor"},
	"rename":     {"refactor"},
	"restructure": {"refactor"},
	"move":       {"refactor"},
	"deploy":     {"deploy"},
	"release":    {"deploy"},
	"ship":       {"deploy"},
}

// NewToolSelector creates a ToolSelector with the given tools.
func NewToolSelector(tools []ToolInfo) *ToolSelector {
	ts := &ToolSelector{
		AllTools:     tools,
		UsageHistory: make(map[string]int),
		TaskPatterns: make(map[string][]string),
	}
	// Seed TaskPatterns from TaskToolMap.
	for k, v := range TaskToolMap {
		ts.TaskPatterns[k] = v
	}
	return ts
}

// Select analyzes the task and returns a ToolSelection with at most maxTools recommended.
func (ts *ToolSelector) Select(task string, maxTools int) *ToolSelection {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	lower := strings.ToLower(task)

	// Determine which intents match the task.
	intentScores := make(map[string]int)
	for keyword, intents := range taskKeywords {
		if strings.Contains(lower, keyword) {
			for _, intent := range intents {
				intentScores[intent]++
			}
		}
	}

	// Collect recommended tool names from matching intents.
	toolScoreMap := make(map[string]int)
	for intent, score := range intentScores {
		if tools, ok := ts.TaskPatterns[intent]; ok {
			for _, t := range tools {
				toolScoreMap[t] += score
			}
		}
	}

	// Boost tools that appear in usage history for similar task words.
	for toolTask, count := range ts.UsageHistory {
		parts := strings.SplitN(toolTask, ":", 2)
		if len(parts) == 2 {
			tool := parts[0]
			taskCtx := strings.ToLower(parts[1])
			// If any word from the recorded task context appears in current task, boost.
			for _, word := range strings.Fields(taskCtx) {
				if len(word) > 3 && strings.Contains(lower, word) {
					toolScoreMap[tool] += count
					break
				}
			}
		}
	}

	// If no intents matched, fall back to a generic set.
	if len(toolScoreMap) == 0 {
		toolScoreMap["Read"] = 1
		toolScoreMap["Grep"] = 1
		toolScoreMap["Bash"] = 1
		toolScoreMap["Edit"] = 1
	}

	// Rank tools by score and pick top maxTools.
	type scoredTool struct {
		name  string
		score int
	}
	var ranked []scoredTool
	for name, score := range toolScoreMap {
		ranked = append(ranked, scoredTool{name, score})
	}
	// Sort descending by score, stable by name for ties.
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].score > ranked[i].score ||
				(ranked[j].score == ranked[i].score && ranked[j].name < ranked[i].name) {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}

	recommended := make([]string, 0, maxTools)
	for i, st := range ranked {
		if i >= maxTools {
			break
		}
		recommended = append(recommended, st.name)
	}

	// Determine excluded tools.
	recommendedSet := make(map[string]bool)
	for _, r := range recommended {
		recommendedSet[r] = true
	}
	var excluded []string
	for _, ti := range ts.AllTools {
		if !recommendedSet[ti.Name] {
			excluded = append(excluded, ti.Name)
		}
	}

	// Build reason string.
	reason := buildReason(intentScores)

	// Confidence based on how many keywords matched.
	totalMatches := 0
	for _, s := range intentScores {
		totalMatches += s
	}
	confidence := 0.5
	if totalMatches >= 3 {
		confidence = 0.95
	} else if totalMatches == 2 {
		confidence = 0.85
	} else if totalMatches == 1 {
		confidence = 0.75
	}

	return &ToolSelection{
		Recommended: recommended,
		Excluded:    excluded,
		Reason:      reason,
		Confidence:  confidence,
	}
}

// buildReason constructs a human-readable reason from the matched intents.
func buildReason(intentScores map[string]int) string {
	if len(intentScores) == 0 {
		return "no clear intent detected — using default tool set"
	}
	// Find the top intent.
	topIntent := ""
	topScore := 0
	for intent, score := range intentScores {
		if score > topScore {
			topScore = score
			topIntent = intent
		}
	}

	switch topIntent {
	case "debug":
		return "debugging task — prioritize file access + execution"
	case "read code":
		return "code reading task — prioritize read-only tools"
	case "write code":
		return "code writing task — prioritize edit + execution tools"
	case "search":
		return "search task — prioritize search + discovery tools"
	case "test":
		return "testing task — prioritize execution + file tools"
	case "review":
		return "review task — prioritize read-only + search tools"
	case "refactor":
		return "refactoring task — prioritize edit + search tools"
	case "deploy":
		return "deployment task — prioritize execution + file tools"
	default:
		return fmt.Sprintf("%s task detected", topIntent)
	}
}

// RecordUsage tracks which tools are used for which tasks.
func (ts *ToolSelector) RecordUsage(tool string, task string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	key := tool + ":" + task
	ts.UsageHistory[key]++
}

// GetRecommendedForIntent returns the tools mapped to a given intent category.
func (ts *ToolSelector) GetRecommendedForIntent(intent string) []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	if tools, ok := ts.TaskPatterns[intent]; ok {
		result := make([]string, len(tools))
		copy(result, tools)
		return result
	}
	return nil
}

// FilterExpensive removes tools marked as "expensive" from the provided list.
func (ts *ToolSelector) FilterExpensive(tools []string) []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	expensiveSet := make(map[string]bool)
	for _, ti := range ts.AllTools {
		if ti.Cost == "expensive" {
			expensiveSet[ti.Name] = true
		}
	}

	var filtered []string
	for _, t := range tools {
		if !expensiveSet[t] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// FormatToolSelection returns a formatted string representation of a ToolSelection.
func FormatToolSelection(task string, selection *ToolSelection) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tool Selection for %q:\n", task))
	sb.WriteString(fmt.Sprintf("  Recommended (%d): %s\n", len(selection.Recommended), strings.Join(selection.Recommended, ", ")))
	sb.WriteString(fmt.Sprintf("  Excluded (%d): %s\n", len(selection.Excluded), strings.Join(selection.Excluded, ", ")))
	sb.WriteString(fmt.Sprintf("  Reason: %s\n", selection.Reason))
	sb.WriteString(fmt.Sprintf("  Confidence: %.2f\n", selection.Confidence))
	return sb.String()
}

// Adapt processes feedback to adjust tool patterns for future selections.
// Feedback format example: "needed WebSearch but it wasn't available"
func (ts *ToolSelector) Adapt(feedback string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	lower := strings.ToLower(feedback)

	// Parse "needed <tool>" pattern.
	var neededTool string
	if idx := strings.Index(lower, "needed "); idx >= 0 {
		rest := feedback[idx+len("needed "):]
		// Take the next word as the tool name.
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			neededTool = fields[0]
		}
	}

	if neededTool == "" {
		return
	}

	// Determine which intents this feedback relates to by checking existing task patterns.
	// Look for exact word match on intent names to avoid ambiguity
	// (e.g., "debug task" should match "debug" not "search" even though it contains "search" substring).
	var relatedIntent string
	words := strings.Fields(lower)
	for intent := range ts.TaskPatterns {
		for _, w := range words {
			if w == intent {
				relatedIntent = intent
				break
			}
		}
		if relatedIntent != "" {
			break
		}
	}

	// Also check keywords to infer intent.
	if relatedIntent == "" {
		for keyword, intents := range taskKeywords {
			if strings.Contains(lower, keyword) && len(intents) > 0 {
				relatedIntent = intents[0]
				break
			}
		}
	}

	if relatedIntent != "" {
		// Add the needed tool to that intent's pattern if not already present.
		tools := ts.TaskPatterns[relatedIntent]
		for _, t := range tools {
			if strings.EqualFold(t, neededTool) {
				return // already present
			}
		}
		ts.TaskPatterns[relatedIntent] = append(ts.TaskPatterns[relatedIntent], neededTool)
	} else {
		// If no intent found, boost the tool in the usage history with a generic marker.
		key := neededTool + ":general"
		ts.UsageHistory[key] += 3
	}
}
