package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ClarifyTool identifies underspecified areas in the active spec and asks
// targeted questions to resolve ambiguity before implementation begins.
type ClarifyTool struct{}

func (ClarifyTool) Name() string      { return "Clarify" }
func (ClarifyTool) Aliases() []string { return []string{"clarify", "spec_clarify", "spec:clarify"} }
func (ClarifyTool) Description() string {
	return "Analyze the active spec for underspecified areas, ambiguities, and missing information. Generates targeted clarification questions that should be resolved before proceeding to implementation. Call this after Specify to refine requirements."
}

func (ClarifyTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"artifact": map[string]interface{}{
				"type":        "string",
				"description": "Which artifact to analyze: spec.md (default), plan.md, or tasks.md",
				"enum":        []string{"spec.md", "plan.md", "tasks.md"},
			},
		},
	}
}

func (ClarifyTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Artifact string `json:"artifact"`
	}
	if input != nil {
		_ = json.Unmarshal(input, &p)
	}
	if p.Artifact == "" {
		p.Artifact = "spec.md"
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, p.Artifact)
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w (call Specify first)", p.Artifact, err)
	}
	content := string(data)
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("%s is empty — write content first", p.Artifact)
	}

	questions := analyzeForClarifications(content, p.Artifact)
	if len(questions) == 0 {
		return fmt.Sprintf("+ %s appears well-specified. No clarification questions generated.", p.Artifact), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d area(s) in %s that need clarification:\n\n", len(questions), p.Artifact)
	for i, q := range questions {
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, q.Category, q.Question)
		if q.Context != "" {
			fmt.Fprintf(&b, "   Context: %s\n", q.Context)
		}
		b.WriteString("\n")
	}
	b.WriteString("Resolve these before proceeding. You can edit the artifact with SpecEdit or re-write it with Specify.")

	return strings.TrimSpace(b.String()), nil
}

type clarification struct {
	Category string
	Question string
	Context  string
}

var (
	clarifyReAmbiguous = regexp.MustCompile(`(?i)\b(maybe|might|could|possibly|perhaps|unclear|TBD|TBD|unknown)\b`)
	clarifyReEdgeCase  = regexp.MustCompile(`(?i)(edge case|error|failure|timeout|invalid|empty|null|missing|race condition)\b`)
	clarifyRePriority  = regexp.MustCompile(`(?i)(must|should|may|optional|required|critical|important|nice.to.have)\b`)
	clarifyReMetrics   = regexp.MustCompile(`(?i)(latency|throughput|capacity|performance|load|scale|concurrent)\b`)
)

func analyzeForClarifications(content, artifact string) []clarification {
	var questions []clarification
	lines := strings.Split(content, "\n")

	// Check for vague/ambiguous language
	for _, line := range lines {
		if clarifyReAmbiguous.MatchString(line) {
			trimmed := strings.TrimSpace(line)
			if len(trimmed) > 80 {
				// Rune-safe truncation: never split a multibyte UTF-8 sequence.
				if runes := []rune(trimmed); len(runes) > 80 {
					trimmed = string(runes[:80]) + "..."
				}
			}
			questions = append(questions, clarification{
				Category: "Ambiguity",
				Question: fmt.Sprintf("Vague language found: %q", trimmed),
				Context:  "Replace with specific, testable language (e.g., 'SHALL respond within 200ms' instead of 'be fast').",
			})
		}
	}

	// Check for missing acceptance criteria
	if !strings.Contains(content, "success") && !strings.Contains(content, "acceptance") && !strings.Contains(content, "criteria") && !strings.Contains(content, "scenario") {
		questions = append(questions, clarification{
			Category: "Acceptance Criteria",
			Question: "No acceptance criteria or success scenarios found",
			Context:  "Add scenarios with WHEN/THEN format to make requirements testable.",
		})
	}

	// Check for missing error handling
	if artifact == "spec.md" && !clarifyReEdgeCase.MatchString(content) {
		questions = append(questions, clarification{
			Category: "Error Handling",
			Question: "No error/edge case scenarios documented",
			Context:  "Consider what happens on invalid input, timeouts, missing data, and concurrent access.",
		})
	}

	// Check for missing scope boundaries
	if !strings.Contains(content, "out of scope") && !strings.Contains(content, "non-goal") && !strings.Contains(content, "boundar") {
		questions = append(questions, clarification{
			Category: "Scope",
			Question: "No explicit scope boundaries defined",
			Context:  "Define what is explicitly OUT of scope to prevent scope creep.",
		})
	}

	// Check for missing priority/ordering
	if artifact == "tasks.md" && !clarifyRePriority.MatchString(content) {
		questions = append(questions, clarification{
			Category: "Priority",
			Question: "Tasks lack priority or dependency ordering",
			Context:  "Add priority labels (must/should/nice-to-have) or dependency ordering to tasks.",
		})
	}

	// Check for missing performance requirements
	if artifact == "spec.md" && clarifyReMetrics.MatchString(content) && !regexp.MustCompile(`(?i)(\d+\s*(ms|s|req/s|%|mb|gb))`).MatchString(content) {
		questions = append(questions, clarification{
			Category: "Measurability",
			Question: "Performance mentioned but no specific metrics defined",
			Context:  "Add concrete numbers (e.g., '< 200ms p95', 'handle 1000 concurrent users').",
		})
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []clarification
	for _, q := range questions {
		key := q.Category + ":" + q.Question
		if !seen[key] {
			seen[key] = true
			unique = append(unique, q)
		}
	}

	// Cap at 10 questions
	if len(unique) > 10 {
		unique = unique[:10]
	}

	return unique
}
