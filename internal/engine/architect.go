package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/provider/routing"
)

// ArchitectConfig configures the two-model architect/editor pipeline.
// A cheap model (architect) plans the changes, then an expensive model (editor)
// executes them precisely. This is a cost-saving pattern: the architect uses
// fewer tokens to plan, allowing the editor to focus on precise implementation.
type ArchitectConfig struct {
	ArchitectModel  string // cheap/fast model for planning, e.g., "haiku"
	EditorModel     string // expensive/precise model for edits, e.g., "sonnet"
	PlanTokenBudget int    // max tokens for architect's plan, default 4096
	Enabled         bool
}

// ArchitectPlan represents the structured output from the architect model.
type ArchitectPlan struct {
	Goal                string
	Steps               []PlanStep
	FilesToModify       []string
	EstimatedComplexity string // "trivial", "simple", "moderate", "complex"
	RawPlan             string
}

// PlanStep is a single step in the architect's plan.
type PlanStep struct {
	Description string
	File        string
	Action      string // "create", "modify", "delete"
	Details     string
}

// Message is a lightweight chat message used by the architect pipeline.
// This avoids coupling to external message types from eyrie.
type ArchitectMessage struct {
	Role    string // "system", "user", "assistant"
	Content string
}

// Architect coordinates the two-model pipeline: a cheap model plans,
// then an expensive model executes each step.
type Architect struct {
	Config ArchitectConfig
	ChatFn func(ctx context.Context, model string, messages []ArchitectMessage) (string, error)
}

const architectSystemPrompt = `You are a software architect. Given a coding task, produce a precise implementation plan.

Output format:
GOAL: <one-line summary>
COMPLEXITY: <trivial|simple|moderate|complex>
FILES: <comma-separated list of files to modify/create>

STEPS:
1. [file.go] MODIFY: <what to change and why>
2. [new_file.go] CREATE: <what this file should contain>
...

Be specific about WHAT to change in each file. The editor will implement your plan exactly.`

// Plan sends the goal and repo context to the architect model and returns a structured plan.
func (a *Architect) Plan(ctx context.Context, goal string, repoContext string) (*ArchitectPlan, error) {
	if a.ChatFn == nil {
		return nil, fmt.Errorf("architect: ChatFn is not configured")
	}

	userContent := goal
	if repoContext != "" {
		userContent = fmt.Sprintf("Repository context:\n%s\n\nTask:\n%s", repoContext, goal)
	}

	messages := []ArchitectMessage{
		{Role: "system", Content: architectSystemPrompt},
		{Role: "user", Content: userContent},
	}

	model := a.Config.ArchitectModel
	if model == "" {
		provider := "anthropic"
		if info, ok := routing.Find(a.Config.EditorModel); ok && info.Provider != "" {
			provider = info.Provider
		}
		model = routing.PreferredModelForTier(provider, routing.TierHaiku, "")
	}

	response, err := a.ChatFn(ctx, model, messages)
	if err != nil {
		return nil, fmt.Errorf("architect: planning failed: %w", err)
	}

	plan, err := ParsePlan(response)
	if err != nil {
		return nil, fmt.Errorf("architect: failed to parse plan: %w", err)
	}

	plan.RawPlan = response
	return plan, nil
}

// ParsePlan extracts GOAL, COMPLEXITY, FILES, and STEPS from the architect's response.
// It handles variations in formatting gracefully.
func ParsePlan(response string) (*ArchitectPlan, error) {
	if strings.TrimSpace(response) == "" {
		return nil, fmt.Errorf("empty plan response")
	}

	plan := &ArchitectPlan{
		RawPlan: response,
	}

	lines := strings.Split(response, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		if strings.HasPrefix(upper, "GOAL:") {
			plan.Goal = strings.TrimSpace(trimmed[len("GOAL:"):])
		} else if strings.HasPrefix(upper, "COMPLEXITY:") {
			complexity := strings.TrimSpace(trimmed[len("COMPLEXITY:"):])
			complexity = strings.ToLower(complexity)
			// Normalize to valid values
			switch complexity {
			case "trivial", "simple", "moderate", "complex":
				plan.EstimatedComplexity = complexity
			default:
				// Try to match partial/variant forms
				if strings.Contains(complexity, "trivial") {
					plan.EstimatedComplexity = "trivial"
				} else if strings.Contains(complexity, "simple") {
					plan.EstimatedComplexity = "simple"
				} else if strings.Contains(complexity, "complex") {
					plan.EstimatedComplexity = "complex"
				} else if strings.Contains(complexity, "moderate") || strings.Contains(complexity, "medium") {
					plan.EstimatedComplexity = "moderate"
				} else {
					plan.EstimatedComplexity = "moderate" // default
				}
			}
		} else if strings.HasPrefix(upper, "FILES:") {
			filesStr := strings.TrimSpace(trimmed[len("FILES:"):])
			if filesStr != "" {
				parts := strings.Split(filesStr, ",")
				for _, p := range parts {
					f := strings.TrimSpace(p)
					if f != "" {
						plan.FilesToModify = append(plan.FilesToModify, f)
					}
				}
			}
		}
	}

	// Parse STEPS section
	plan.Steps = parseSteps(lines)

	// If no goal was explicitly found, use the first non-empty line as a fallback
	if plan.Goal == "" && len(lines) > 0 {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				plan.Goal = trimmed
				break
			}
		}
	}

	// If no complexity was set, default to moderate
	if plan.EstimatedComplexity == "" {
		plan.EstimatedComplexity = "moderate"
	}

	return plan, nil
}

// parseSteps extracts step entries from lines. It looks for numbered steps
// that follow patterns like:
//
//  1. [file.go] ACTION: description
//  2. [file.go] ACTION: description
//
// It also handles variants without brackets or with different numbering.
func parseSteps(lines []string) []PlanStep {
	var steps []PlanStep
	inSteps := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		// Detect start of STEPS section
		if strings.HasPrefix(upper, "STEPS:") || strings.HasPrefix(upper, "STEPS") && strings.HasSuffix(upper, ":") {
			inSteps = true
			continue
		}

		if !inSteps {
			continue
		}

		// Skip empty lines within steps
		if trimmed == "" {
			continue
		}

		// Try to parse a step line
		step, ok := parseStepLine(trimmed)
		if ok {
			steps = append(steps, step)
		}
	}

	return steps
}

// parseStepLine attempts to parse a single step line.
// Supported formats:
//   - "1. [file.go] MODIFY: description"
//   - "- [file.go] CREATE: description"
//   - "1. file.go - MODIFY: description"
//   - "1. MODIFY file.go: description"
func parseStepLine(line string) (PlanStep, bool) {
	// Strip leading number/bullet: "1. ", "- ", "* "
	stripped := line
	for i, ch := range stripped {
		if ch == '.' || ch == ')' {
			stripped = strings.TrimSpace(stripped[i+1:])
			break
		}
		if ch == '-' || ch == '*' {
			if i == 0 {
				stripped = strings.TrimSpace(stripped[1:])
				break
			}
		}
		if ch < '0' || ch > '9' {
			break
		}
	}

	step := PlanStep{}

	// Try format: [file.go] ACTION: description
	if strings.HasPrefix(stripped, "[") {
		endBracket := strings.Index(stripped, "]")
		if endBracket > 1 {
			step.File = strings.TrimSpace(stripped[1:endBracket])
			rest := strings.TrimSpace(stripped[endBracket+1:])
			action, details := extractActionAndDetails(rest)
			step.Action = action
			if details != "" {
				step.Details = details
				step.Description = fmt.Sprintf("%s %s: %s", step.Action, step.File, details)
			} else {
				step.Description = fmt.Sprintf("%s %s", step.Action, step.File)
			}
			return step, true
		}
	}

	// Try format: ACTION file.go: description or ACTION: file.go - description
	upperStripped := strings.ToUpper(stripped)
	for _, action := range []string{"CREATE", "MODIFY", "DELETE"} {
		if strings.HasPrefix(upperStripped, action) {
			rest := strings.TrimSpace(stripped[len(action):])
			// Remove leading colon or dash
			rest = strings.TrimLeft(rest, ":- ")
			rest = strings.TrimSpace(rest)
			// Next token is likely the filename
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) >= 1 {
				// Clean file name (strip trailing colon/dash)
				step.File = strings.TrimRight(parts[0], ":-")
				step.Action = strings.ToLower(action)
				if len(parts) == 2 {
					step.Details = strings.TrimLeft(parts[1], ":- ")
				}
				step.Description = fmt.Sprintf("%s %s", step.Action, step.File)
				if step.Details != "" {
					step.Description += ": " + step.Details
				}
				return step, true
			}
		}
	}

	// Fallback: treat the whole line as a step with unknown action
	if stripped != "" {
		step.Description = stripped
		step.Action = "modify"
		// Try to extract a filename (anything ending in common extensions)
		words := strings.Fields(stripped)
		for _, w := range words {
			clean := strings.Trim(w, "[](),:")
			if looksLikeFile(clean) {
				step.File = clean
				break
			}
		}
		return step, true
	}

	return PlanStep{}, false
}

// looksLikeFile returns true if the string looks like a filename.
func looksLikeFile(s string) bool {
	extensions := []string{
		".go", ".js", ".ts", ".py", ".rs", ".java", ".c", ".h",
		".cpp", ".rb", ".md", ".yaml", ".yml", ".json", ".toml", ".sql", ".sh",
	}
	lower := strings.ToLower(s)
	for _, ext := range extensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// extractActionAndDetails splits "ACTION: details" into the action and details.
func extractActionAndDetails(s string) (string, string) {
	s = strings.TrimSpace(s)
	// Check for known actions
	upper := strings.ToUpper(s)
	for _, action := range []string{"CREATE", "MODIFY", "DELETE"} {
		if strings.HasPrefix(upper, action) {
			rest := strings.TrimSpace(s[len(action):])
			rest = strings.TrimLeft(rest, ":- ")
			return strings.ToLower(action), strings.TrimSpace(rest)
		}
	}
	// No recognized action; default to "modify"
	return "modify", s
}

// ShouldUseArchitect applies heuristics to decide whether the architect/editor
// pipeline should be used for this request. It returns true for complex tasks
// that benefit from planning.
func ShouldUseArchitect(prompt string, messageCount int) bool {
	lower := strings.ToLower(prompt)

	// Don't use for explicit speed requests
	if strings.Contains(lower, "quick") || strings.Contains(lower, "fast") ||
		strings.Contains(lower, "just do it") || strings.Contains(lower, "hurry") {
		return false
	}

	// Don't use for simple questions
	if strings.HasPrefix(lower, "what ") || strings.HasPrefix(lower, "how ") ||
		strings.HasPrefix(lower, "why ") || strings.HasPrefix(lower, "explain ") {
		words := strings.Fields(prompt)
		if len(words) <= 15 {
			return false
		}
	}

	// Use for multi-file references
	if strings.Contains(lower, "files") || strings.Contains(lower, "across") ||
		strings.Contains(lower, "multiple") || strings.Contains(lower, "all of") {
		return true
	}

	// Use for complex task indicators
	if strings.Contains(lower, "refactor") || strings.Contains(lower, "redesign") ||
		strings.Contains(lower, "implement") || strings.Contains(lower, "architecture") ||
		strings.Contains(lower, "migrate") || strings.Contains(lower, "restructure") {
		return true
	}

	// Word count heuristic: complex prompts tend to be longer
	words := strings.Fields(prompt)
	if len(words) > 50 {
		return true
	}

	// Early in conversation and long prompt suggests initial complex task
	if messageCount <= 2 && len(words) > 30 {
		return true
	}

	return false
}

// BuildEditorPrompt formats a focused prompt for the editor model to implement
// a specific step from the architect's plan.
func BuildEditorPrompt(plan *ArchitectPlan, step PlanStep) string {
	var b strings.Builder
	b.WriteString("Implement this specific step from the plan:\n\n")
	b.WriteString(fmt.Sprintf("Overall goal: %s\n\n", plan.Goal))
	b.WriteString(fmt.Sprintf("Your task: %s\n", step.Description))
	b.WriteString(fmt.Sprintf("File: %s\n", step.File))
	b.WriteString(fmt.Sprintf("Action: %s\n", step.Action))
	if step.Details != "" {
		b.WriteString(fmt.Sprintf("Details: %s\n", step.Details))
	}
	b.WriteString("\nImplement this precisely. Do not deviate from the plan.")
	return b.String()
}

// EstimateSavings calculates the estimated cost savings of using the architect/editor
// pipeline versus using the expensive model for everything. The architectCost and
// editorCost are per-million-token input prices.
func EstimateSavings(plan *ArchitectPlan, architectCost, editorCost float64) string {
	if plan == nil || len(plan.Steps) == 0 {
		return "No savings estimate available (empty plan)."
	}

	// Estimate tokens: architect plan ~1000 tokens, each editor step ~2000 tokens
	planTokens := 1000.0
	editorTokensPerStep := 2000.0
	totalEditorTokens := float64(len(plan.Steps)) * editorTokensPerStep

	// Cost with architect pipeline
	architectPipelineCost := (planTokens * architectCost / 1_000_000) +
		(totalEditorTokens * editorCost / 1_000_000)

	// Cost without pipeline (all tokens at editor price)
	totalTokensWithout := planTokens + totalEditorTokens
	withoutPipelineCost := totalTokensWithout * editorCost / 1_000_000

	savings := withoutPipelineCost - architectPipelineCost
	if savings <= 0 {
		return fmt.Sprintf("No cost savings (architect pipeline costs the same or more). Pipeline: $%.6f, Direct: $%.6f",
			architectPipelineCost, withoutPipelineCost)
	}

	percentSaved := (savings / withoutPipelineCost) * 100
	return fmt.Sprintf("Estimated savings: $%.6f (%.1f%% reduction). Pipeline: $%.6f vs Direct: $%.6f",
		savings, percentSaved, architectPipelineCost, withoutPipelineCost)
}
