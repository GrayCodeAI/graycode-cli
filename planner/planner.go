// Package planner implements structured planning that generates artifacts before
// coding begins. It produces a Plan containing tasks, design notes, and risk
// analysis, and can persist/load plans as JSON files under .hawk/plans/.
package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Plan represents a structured development plan.
type Plan struct {
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	Tasks     []Task    `json:"tasks"`
	Design    string    `json:"design"`     // high-level design notes
	RiskNotes string    `json:"risk_notes"` // identified risks and mitigations
	CreatedAt time.Time `json:"created_at"`
}

// Task represents a single unit of work within a plan.
type Task struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	File        string `json:"file,omitempty"`    // target file
	Status      string `json:"status"`            // pending, done, skipped
	Depends     []int  `json:"depends,omitempty"` // IDs of tasks this depends on
}

// PlanPrompt contains the prompt to send to the LLM for plan generation.
type PlanPrompt struct {
	System string
	User   string
}

// Generate creates a PlanPrompt from a feature description and repository context.
// This is a template -- the actual LLM call happens in hawk's engine.
func Generate(description string, repoContext string) *PlanPrompt {
	system := `You are a senior software architect. Given a feature description and repository context, produce a structured development plan in JSON format.

The plan must include:
- "title": A concise title for the plan (under 60 chars)
- "summary": A 1-3 sentence summary of the feature
- "tasks": An ordered list of tasks, each with:
  - "id": sequential integer starting at 1
  - "description": what to do
  - "file": target file path (if applicable)
  - "status": always "pending" for new plans
  - "depends": list of task IDs this depends on (empty if none)
- "design": High-level design notes explaining the approach
- "risk_notes": Potential risks and mitigations

Respond ONLY with valid JSON matching the schema above. Do not wrap in markdown code fences.`

	user := fmt.Sprintf("Feature description:\n%s\n\nRepository context:\n%s", description, repoContext)

	return &PlanPrompt{
		System: system,
		User:   user,
	}
}

// ParsePlan parses the LLM response into a structured Plan.
// It handles responses that may be wrapped in markdown code fences.
func ParsePlan(response string) (*Plan, error) {
	// Strip markdown code fences if present
	cleaned := strings.TrimSpace(response)
	if strings.HasPrefix(cleaned, "```") {
		// Remove opening fence (with optional language tag)
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	var plan Plan
	if err := json.Unmarshal([]byte(cleaned), &plan); err != nil {
		return nil, fmt.Errorf("planner: failed to parse plan JSON: %w", err)
	}

	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}

	// Validate and set defaults for tasks
	for i := range plan.Tasks {
		if plan.Tasks[i].Status == "" {
			plan.Tasks[i].Status = "pending"
		}
		if plan.Tasks[i].ID == 0 {
			plan.Tasks[i].ID = i + 1
		}
	}

	return &plan, nil
}

// Save writes the plan to .hawk/plans/{sanitized-title}.json under the given dir.
// Returns the path of the saved file.
func Save(dir string, plan *Plan) (string, error) {
	plansDir := filepath.Join(dir, ".hawk", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		return "", fmt.Errorf("planner: cannot create plans directory: %w", err)
	}

	filename := sanitizeFilename(plan.Title) + ".json"
	path := filepath.Join(plansDir, filename)

	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return "", fmt.Errorf("planner: failed to marshal plan: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("planner: failed to write plan file: %w", err)
	}

	return path, nil
}

// Load reads a plan from disk.
func Load(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("planner: failed to read plan file: %w", err)
	}

	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("planner: failed to parse plan file: %w", err)
	}

	return &plan, nil
}

// MarkDone marks a task as completed.
// If the taskID is not found, this is a no-op.
func MarkDone(plan *Plan, taskID int) {
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == taskID {
			plan.Tasks[i].Status = "done"
			return
		}
	}
}

// MarkSkipped marks a task as skipped.
// If the taskID is not found, this is a no-op.
func MarkSkipped(plan *Plan, taskID int) {
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == taskID {
			plan.Tasks[i].Status = "skipped"
			return
		}
	}
}

// PendingTasks returns all tasks that are still pending.
func PendingTasks(plan *Plan) []Task {
	var pending []Task
	for _, t := range plan.Tasks {
		if t.Status == "pending" {
			pending = append(pending, t)
		}
	}
	return pending
}

// FormatMarkdown renders the plan as readable markdown.
func FormatMarkdown(plan *Plan) string {
	var b strings.Builder

	b.WriteString("# ")
	b.WriteString(plan.Title)
	b.WriteString("\n\n")

	if plan.Summary != "" {
		b.WriteString(plan.Summary)
		b.WriteString("\n\n")
	}

	if plan.Design != "" {
		b.WriteString("## Design\n\n")
		b.WriteString(plan.Design)
		b.WriteString("\n\n")
	}

	if len(plan.Tasks) > 0 {
		b.WriteString("## Tasks\n\n")
		for _, task := range plan.Tasks {
			checkbox := "[ ]"
			if task.Status == "done" {
				checkbox = "[x]"
			} else if task.Status == "skipped" {
				checkbox = "[-]"
			}

			b.WriteString(fmt.Sprintf("- %s **%d.** %s", checkbox, task.ID, task.Description))
			if task.File != "" {
				b.WriteString(fmt.Sprintf(" (`%s`)", task.File))
			}
			if len(task.Depends) > 0 {
				deps := make([]string, len(task.Depends))
				for i, d := range task.Depends {
					deps[i] = fmt.Sprintf("#%d", d)
				}
				b.WriteString(fmt.Sprintf(" [depends: %s]", strings.Join(deps, ", ")))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if plan.RiskNotes != "" {
		b.WriteString("## Risks\n\n")
		b.WriteString(plan.RiskNotes)
		b.WriteString("\n\n")
	}

	b.WriteString(fmt.Sprintf("*Created: %s*\n", plan.CreatedAt.Format("2006-01-02 15:04")))

	return b.String()
}

// sanitizeFilename converts a title into a safe filename.
func sanitizeFilename(title string) string {
	// Replace spaces and unsafe characters with hyphens
	replacer := strings.NewReplacer(
		" ", "-",
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "",
		"?", "",
		"\"", "",
		"<", "",
		">", "",
		"|", "",
	)
	name := replacer.Replace(strings.TrimSpace(title))
	name = strings.ToLower(name)

	// Collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	// Trim leading/trailing hyphens
	name = strings.Trim(name, "-")

	if name == "" {
		name = "untitled-plan"
	}

	// Limit length
	if len(name) > 100 {
		name = name[:100]
	}

	return name
}
