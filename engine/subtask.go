package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// Subtask represents a single unit of work within a plan.
type Subtask struct {
	ID          int
	Title       string
	Description string
	Files       []string
	Status      string // "pending", "in_progress", "done", "skipped"
}

// PlanState tracks progress through a set of subtasks.
type PlanState struct {
	Name     string
	Subtasks []Subtask
	Current  int
	Active   bool
}

// NewPlanState creates a new plan with the given name and no subtasks.
func NewPlanState(name string) *PlanState {
	return &PlanState{
		Name:   name,
		Active: true,
	}
}

// DecomposePrompt returns a system prompt that instructs the LLM to break a task
// into numbered subtasks with titles, descriptions, and file lists.
func DecomposePrompt() string {
	return `Break the following task into numbered subtasks. For each subtask, provide:
1. A short title on the first line (prefixed with the number and a period)
2. A description indented on the next line
3. A list of relevant files indented and prefixed with "Files:"

Use this exact format:

1. Title here
   Description of what to do
   Files: path/to/file.go, another/file.go

2. Another title
   Another description
   Files: third.go

Keep subtasks focused and actionable. Each subtask should represent a single logical unit of work.`
}

// ParseSubtasks parses LLM output formatted as numbered subtasks.
// Expected format:
//
//	1. Title here
//	   Description of what to do
//	   Files: path/to/file.go, another/file.go
func ParseSubtasks(output string) []Subtask {
	lines := strings.Split(output, "\n")
	var subtasks []Subtask
	var current *Subtask

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if current != nil {
				subtasks = append(subtasks, *current)
				current = nil
			}
			continue
		}

		// Check for numbered title line: "1. Title" or "2. Title"
		if id, title, ok := parseNumberedLine(trimmed); ok {
			if current != nil {
				subtasks = append(subtasks, *current)
			}
			current = &Subtask{
				ID:     id,
				Title:  title,
				Status: "pending",
			}
			continue
		}

		if current == nil {
			continue
		}

		// Check for Files: line
		if strings.HasPrefix(trimmed, "Files:") {
			filesStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "Files:"))
			if filesStr != "" {
				parts := strings.Split(filesStr, ",")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						current.Files = append(current.Files, p)
					}
				}
			}
			continue
		}

		// Otherwise it's a description line
		if current.Description == "" {
			current.Description = trimmed
		} else {
			current.Description += " " + trimmed
		}
	}

	// Don't forget the last subtask
	if current != nil {
		subtasks = append(subtasks, *current)
	}

	return subtasks
}

// parseNumberedLine checks if a line matches "N. Title" and returns the number and title.
func parseNumberedLine(line string) (int, string, bool) {
	dotIdx := strings.Index(line, ".")
	if dotIdx <= 0 || dotIdx > 4 {
		return 0, "", false
	}

	numStr := line[:dotIdx]
	id, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, "", false
	}

	title := strings.TrimSpace(line[dotIdx+1:])
	if title == "" {
		return 0, "", false
	}

	return id, title, true
}

// Next returns the next pending subtask and marks it as in_progress, or nil if none remain.
func (ps *PlanState) Next() *Subtask {
	for i := range ps.Subtasks {
		if ps.Subtasks[i].Status == "pending" {
			ps.Subtasks[i].Status = "in_progress"
			ps.Current = ps.Subtasks[i].ID
			return &ps.Subtasks[i]
		}
	}
	return nil
}

// MarkDone sets the subtask with the given ID to "done".
func (ps *PlanState) MarkDone(id int) {
	for i := range ps.Subtasks {
		if ps.Subtasks[i].ID == id {
			ps.Subtasks[i].Status = "done"
			return
		}
	}
}

// Skip sets the subtask with the given ID to "skipped".
func (ps *PlanState) Skip(id int) {
	for i := range ps.Subtasks {
		if ps.Subtasks[i].ID == id {
			ps.Subtasks[i].Status = "skipped"
			return
		}
	}
}

// Format returns a human-readable display of the plan state.
func (ps *PlanState) Format() string {
	if len(ps.Subtasks) == 0 {
		return fmt.Sprintf("Plan: %s (no subtasks)", ps.Name)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Plan: %s\n", ps.Name))
	b.WriteString(fmt.Sprintf("%s\n\n", ps.Progress()))

	for _, st := range ps.Subtasks {
		icon := statusIcon(st.Status)
		b.WriteString(fmt.Sprintf("  %s %d. %s\n", icon, st.ID, st.Title))
		if st.Description != "" {
			b.WriteString(fmt.Sprintf("     %s\n", st.Description))
		}
		if len(st.Files) > 0 {
			b.WriteString(fmt.Sprintf("     Files: %s\n", strings.Join(st.Files, ", ")))
		}
	}
	return b.String()
}

// Progress returns a short progress string like "3/7 subtasks complete".
func (ps *PlanState) Progress() string {
	done := 0
	for _, st := range ps.Subtasks {
		if st.Status == "done" {
			done++
		}
	}
	return fmt.Sprintf("%d/%d subtasks complete", done, len(ps.Subtasks))
}

func statusIcon(status string) string {
	switch status {
	case "done":
		return "[x]"
	case "in_progress":
		return "[>]"
	case "skipped":
		return "[-]"
	default:
		return "[ ]"
	}
}
