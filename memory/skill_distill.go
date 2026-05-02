package memory

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SkillDistiller extracts reusable skills from successful task completions.
type SkillDistiller struct{}

// DistilledSkill represents a reusable skill extracted from a completed task.
type DistilledSkill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Tools       []string `json:"tools"`
	Patterns    []string `json:"patterns"` // reusable patterns discovered
}

// BuildSkillPrompt creates a prompt that asks the LLM to distill a completed task
// into a reusable skill document.
func (sd *SkillDistiller) BuildSkillPrompt(taskDescription string, toolsUsed []string, filesModified []string, outcome string) string {
	return fmt.Sprintf(`You are a skill extraction agent. Analyze this completed task and distill it into a reusable skill.

<task>%s</task>
<tools_used>%s</tools_used>
<files_modified>%s</files_modified>
<outcome>%s</outcome>

Extract:
1. What was the task?
2. What steps were taken?
3. What tools were used and in what order?
4. What patterns emerged that could be reused?
5. What would you do differently next time?

Respond with ONLY a JSON object:
{
  "name": "short skill name",
  "description": "what this skill accomplishes",
  "steps": ["step 1", "step 2"],
  "tools": ["Tool1", "Tool2"],
  "patterns": ["pattern 1", "pattern 2"]
}`,
		taskDescription,
		strings.Join(toolsUsed, ", "),
		strings.Join(filesModified, ", "),
		outcome,
	)
}

// ParseSkill extracts the skill from the LLM response.
func (sd *SkillDistiller) ParseSkill(llmResponse string) (*DistilledSkill, error) {
	start := strings.Index(llmResponse, "{")
	end := strings.LastIndex(llmResponse, "}")
	if start < 0 || end < 0 || end <= start {
		return nil, fmt.Errorf("no JSON object found in response")
	}
	var skill DistilledSkill
	if err := json.Unmarshal([]byte(llmResponse[start:end+1]), &skill); err != nil {
		return nil, fmt.Errorf("invalid skill JSON: %w", err)
	}
	if skill.Name == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	return &skill, nil
}
