// Package prompts manages workspace context for hawk sessions.
// This file implements auto-accumulation of learnings into .hawk/agents.md.
package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AgentsAccumulator captures learnings from successful edits and appends
// them to .hawk/agents.md for future sessions to benefit from.
type AgentsAccumulator struct {
	projectDir string
	filePath   string
	mu         sync.Mutex
	buffer     []Learning
}

// Learning represents a captured insight from the current session.
type Learning struct {
	Timestamp time.Time
	Context   string   // what was being done
	Pattern   string   // what was learned
	Files     []string // files involved
}

// NewAgentsAccumulator creates an accumulator for the given project directory.
func NewAgentsAccumulator(projectDir string) *AgentsAccumulator {
	return &AgentsAccumulator{
		projectDir: projectDir,
		filePath:   filepath.Join(projectDir, ".hawk", "agents.md"),
	}
}

// Record captures a learning from a successful edit.
func (a *AgentsAccumulator) Record(context string, pattern string, files []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.buffer = append(a.buffer, Learning{
		Timestamp: time.Now(),
		Context:   context,
		Pattern:   pattern,
		Files:     files,
	})
}

// Flush writes all buffered learnings to .hawk/agents.md.
func (a *AgentsAccumulator) Flush() error {
	a.mu.Lock()
	learnings := make([]Learning, len(a.buffer))
	copy(learnings, a.buffer)
	a.buffer = nil
	a.mu.Unlock()

	if len(learnings) == 0 {
		return nil
	}

	// Ensure .hawk directory exists
	dir := filepath.Dir(a.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating .hawk directory: %w", err)
	}

	// Read existing content
	existing, _ := os.ReadFile(a.filePath)
	content := string(existing)

	// Add header if file is new
	if content == "" {
		content = "# Agent Learnings\n\n"
		content += "Auto-captured patterns and conventions discovered during coding sessions.\n\n"
	}

	// Append new learnings
	var sb strings.Builder
	sb.WriteString(content)

	if !strings.HasSuffix(content, "\n\n") {
		sb.WriteString("\n")
	}

	for _, l := range learnings {
		sb.WriteString(fmt.Sprintf("## %s\n\n", l.Timestamp.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("**Context:** %s\n\n", l.Context))
		sb.WriteString(fmt.Sprintf("**Pattern:** %s\n\n", l.Pattern))
		if len(l.Files) > 0 {
			sb.WriteString(fmt.Sprintf("**Files:** %s\n\n", strings.Join(l.Files, ", ")))
		}
		sb.WriteString("---\n\n")
	}

	// Write back
	if err := os.WriteFile(a.filePath, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("writing agents.md: %w", err)
	}

	return nil
}

// ForPrompt returns the accumulated learnings formatted for injection into
// the system prompt. Returns empty string if no learnings exist.
func (a *AgentsAccumulator) ForPrompt(maxEntries int) string {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := os.ReadFile(a.filePath)
	if err != nil {
		return ""
	}

	content := string(data)
	if content == "" {
		return ""
	}

	// Parse sections
	sections := parseLearningSections(content)
	if len(sections) == 0 {
		return ""
	}

	// Limit to recent entries
	if maxEntries > 0 && len(sections) > maxEntries {
		sections = sections[len(sections)-maxEntries:]
	}

	var sb strings.Builder
	sb.WriteString("## Project Learnings (from previous sessions)\n\n")
	for _, s := range sections {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	return sb.String()
}

// parseLearningSections splits the agents.md content into individual learning sections.
func parseLearningSections(content string) []string {
	var sections []string
	var current strings.Builder

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## 20") && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}

	if current.Len() > 0 {
		sections = append(sections, current.String())
	}

	return sections
}

// ExtractPattern analyzes a successful edit and extracts a reusable pattern.
// This is a simple heuristic - in practice, the LLM would generate this.
func ExtractPattern(toolName string, filePath string, diff string) string {
	// Simple pattern extraction based on common operations
	switch {
	case toolName == "Write" && strings.HasSuffix(filePath, "_test.go"):
		return "Created test file for " + filepath.Base(filePath)
	case toolName == "Edit" && strings.Contains(diff, "func Test"):
		return "Modified test in " + filepath.Base(filePath)
	case toolName == "Write" && strings.HasSuffix(filePath, ".md"):
		return "Updated documentation: " + filepath.Base(filePath)
	case toolName == "Edit" && strings.Contains(diff, "import"):
		return "Updated imports in " + filepath.Base(filePath)
	default:
		return fmt.Sprintf("Modified %s via %s", filepath.Base(filePath), toolName)
	}
}
