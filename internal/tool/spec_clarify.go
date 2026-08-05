package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/spec"
)

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

	content := readFileStr(filepath.Join(dir, p.Artifact))
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("empty %s — cannot analyze", p.Artifact)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Clarify: %s\n\n", p.Artifact)

	questions := analyzeAmbiguity(p.Artifact, content)

	if len(questions) == 0 {
		b.WriteString("No ambiguities or underspecified areas detected. Safe to advance.\n")
		return strings.TrimSpace(b.String()), nil
	}

	fmt.Fprintf(&b, "**%d clarification questions found**\n\n", len(questions))
	for i, q := range questions {
		fmt.Fprintf(&b, "%d. **%s**: %s\n", i+1, q.Category, q.Question)
		if q.Context != "" {
			fmt.Fprintf(&b, "   - Context: %s\n", q.Context)
		}
	}

	return strings.TrimSpace(b.String()), nil
}

type SpecClarifyTool struct{}

func (SpecClarifyTool) Name() string { return "SpecClarify" }
func (SpecClarifyTool) Aliases() []string {
	return []string{"spec_clarify_phase", "spec:clarify_phase"}
}

func (SpecClarifyTool) Description() string {
	return "Resolve ambiguities before advancing to the next spec phase. Analyzes proposal/spec for unclear requirements, missing context, and unstated assumptions. Returns targeted questions that must be answered before proceeding."
}

func (SpecClarifyTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"phase": map[string]interface{}{
				"type":        "string",
				"description": "Phase to clarify for: proposal, spec, design, plan, tasks",
				"enum":        []string{"proposal", "spec", "design", "plan", "tasks"},
			},
			"auto_resolve": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, attempt to resolve ambiguities using codebase context",
			},
		},
		"required": []string{"phase"},
	}
}

type ClarifyQuestion struct {
	Category string `json:"category"`
	Question string `json:"question"`
	Context  string `json:"context"`
	Answer   string `json:"answer,omitempty"`
	Resolved bool   `json:"resolved"`
}

func (SpecClarifyTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Phase       string `json:"phase"`
		AutoResolve bool   `json:"auto_resolve"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}

	dir, err := specDir(ctx)
	if err != nil {
		return "", err
	}

	content := ""
	switch p.Phase {
	case "proposal":
		content = readFileStr(filepath.Join(dir, "proposal.md"))
	case "spec":
		content = readFileStr(filepath.Join(dir, "spec.md"))
	case "design":
		content = readFileStr(filepath.Join(dir, "design.md"))
	case "plan":
		content = readFileStr(filepath.Join(dir, "plan.md"))
	case "tasks":
		content = readFileStr(filepath.Join(dir, "tasks.md"))
	}

	if content == "" {
		return fmt.Sprintf("No %s content found. Write the artifact first.", p.Phase), nil
	}

	questions := analyzeAmbiguity(p.Phase, content)

	if p.AutoResolve {
		for i := range questions {
			if answer := attemptResolution(questions[i], dir); answer != "" {
				questions[i].Answer = answer
				questions[i].Resolved = true
			}
		}
	}

	unresolved := 0
	for _, q := range questions {
		if !q.Resolved {
			unresolved++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## Clarify Phase: %s\n\n", strings.Title(p.Phase))
	fmt.Fprintf(&b, "**%d questions found, %d unresolved**\n\n", len(questions), unresolved)

	if unresolved > 0 {
		b.WriteString("### Questions to Resolve\n\n")
		n := 0
		for _, q := range questions {
			if q.Resolved {
				continue
			}
			n++
			fmt.Fprintf(&b, "%d. **%s**: %s\n", n, q.Category, q.Question)
			if q.Context != "" {
				fmt.Fprintf(&b, "   - Context: %s\n", q.Context)
			}
		}
		b.WriteString("\n")
	}

	if unresolved == 0 && len(questions) > 0 {
		b.WriteString("All questions resolved. Safe to advance.\n\n")
	} else if len(questions) == 0 {
		b.WriteString("No ambiguities detected. Safe to advance.\n\n")
	}

	clarifyPath := filepath.Join(dir, ".clarify.json")
	if data, err := json.MarshalIndent(questions, "", "  "); err == nil {
		_ = os.WriteFile(clarifyPath, data, 0o600)
	}

	return strings.TrimSpace(b.String()), nil
}

func analyzeAmbiguity(phase, content string) []ClarifyQuestion {
	var questions []ClarifyQuestion
	lower := strings.ToLower(content)

	reAmbiguity := regexp.MustCompile(`(?i)\b(tbd|todo|maybe|might|could|possibly|unsure|unclear|figure out|decide later)\b`)
	for _, match := range reAmbiguity.FindAllString(content, -1) {
		questions = append(questions, ClarifyQuestion{
			Category: "Ambiguity",
			Question: fmt.Sprintf("Resolve vague term: %q", match),
			Context:  extractContext(content, match),
		})
	}

	reNeedsClarify := regexp.MustCompile(`\[NEEDS CLARIFICATION:\s*(.+?)\]`)
	for _, match := range reNeedsClarify.FindAllStringSubmatch(content, -1) {
		questions = append(questions, ClarifyQuestion{
			Category: "Unresolved",
			Question: match[1],
			Context:  "Explicit [NEEDS CLARIFICATION] marker found",
		})
	}

	switch phase {
	case "proposal":
		if !strings.Contains(lower, "out of scope") && !strings.Contains(lower, "non-goal") {
			questions = append(questions, ClarifyQuestion{
				Category: "Scope",
				Question: "What is explicitly out of scope for this change?",
				Context:  "No scope boundary defined",
			})
		}
		if !strings.Contains(lower, "success criteria") {
			questions = append(questions, ClarifyQuestion{
				Category: "Success Criteria",
				Question: "How will we know this change is complete and correct?",
				Context:  "No success criteria defined",
			})
		}
		if !strings.Contains(lower, "breaking") {
			questions = append(questions, ClarifyQuestion{
				Category: "Compatibility",
				Question: "Does this change break any existing APIs or behaviors?",
				Context:  "No backward compatibility assessment",
			})
		}
	case "spec":
		reqs := spec.ExtractReqIDs(content)
		if len(reqs) == 0 {
			questions = append(questions, ClarifyQuestion{
				Category: "Traceability",
				Question: "No REQ-XXX identifiers found. Add requirement IDs for code traceability?",
				Context:  "Requirements lack machine-readable identifiers",
			})
		}
		for _, req := range reqs {
			if !strings.Contains(lower, "shall") && !strings.Contains(lower, "when") {
				questions = append(questions, ClarifyQuestion{
					Category: "EARS Notation",
					Question: fmt.Sprintf("Requirement %s: Use EARS notation (The system shall / WHEN...THEN / SHALL NOT)", req.Raw),
					Context:  "Requirement lacks structured acceptance criteria",
				})
			}
		}
	case "design":
		if !strings.Contains(lower, "architecture") && !strings.Contains(lower, "component") {
			questions = append(questions, ClarifyQuestion{
				Category: "Architecture",
				Question: "What is the high-level architecture? What are the key components?",
				Context:  "No architectural description found",
			})
		}
		if !strings.Contains(lower, "risk") && !strings.Contains(lower, "trade-off") {
			questions = append(questions, ClarifyQuestion{
				Category: "Risk Assessment",
				Question: "What are the key risks and trade-offs of this design?",
				Context:  "No risk assessment documented",
			})
		}
	case "plan":
		if !strings.Contains(lower, "simplicity") {
			questions = append(questions, ClarifyQuestion{
				Category: "Simplicity Gate",
				Question: "Does the plan use <=3 projects for initial implementation?",
				Context:  "Simplicity gate not documented",
			})
		}
		if !strings.Contains(lower, "anti-abstraction") {
			questions = append(questions, ClarifyQuestion{
				Category: "Anti-Abstraction Gate",
				Question: "Does the plan use framework features directly (no wrappers)?",
				Context:  "Anti-abstraction gate not documented",
			})
		}
	case "tasks":
		tasks := spec.ParseTasks(content)
		for _, t := range tasks {
			if len(t.Files) == 0 {
				questions = append(questions, ClarifyQuestion{
					Category: "Task Scope",
					Question: fmt.Sprintf("Task %q: Which files will be modified?", t.Description),
					Context:  "No file scope defined for task",
				})
			}
		}
	}

	return questions
}

func attemptResolution(q ClarifyQuestion, dir string) string {
	if q.Category == "Scope" {
		return ""
	}
	if q.Category == "Success Criteria" {
		return ""
	}
	return ""
}

func extractContext(content, match string) string {
	idx := strings.Index(content, match)
	if idx < 0 {
		return ""
	}
	start := idx - 40
	if start < 0 {
		start = 0
	}
	end := idx + len(match) + 40
	if end > len(content) {
		end = len(content)
	}
	return "..." + strings.TrimSpace(content[start:end]) + "..."
}

func init() {
	_ = SpecClarifyTool{}
}
