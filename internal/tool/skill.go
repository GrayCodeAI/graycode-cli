package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/plugin"
)

type SkillTool struct{}

func (SkillTool) Name() string      { return "Skill" }
func (SkillTool) Aliases() []string { return []string{"skill"} }
func (SkillTool) Description() string {
	return "Load instructions from a local Hawk skill. Use without a skill name to list available skills."
}

func (SkillTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"skill": map[string]interface{}{"type": "string", "description": "Skill name to load"},
		},
	}
}

func (SkillTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Skill string `json:"skill"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
	}

	cwd, _ := os.Getwd()
	if p.Skill == "" {
		allSkills, err := plugin.DefaultRegistry.List(ctx, cwd)
		if err != nil {
			return "", err
		}
		var modelSkills []plugin.SkillEntry
		for _, s := range allSkills {
			if s.Invocation.IsModelInvocable() {
				modelSkills = append(modelSkills, s)
			}
		}

		if len(modelSkills) == 0 {
			return "No skills found in Hawk user state, .agents/skills, or .codex/skills.", nil
		}
		names := make([]string, 0, len(modelSkills))
		for _, s := range modelSkills {
			names = append(names, s.Name)
		}
		sort.Strings(names)
		return "Available skills:\n" + strings.Join(names, "\n"), nil
	}

	entry, err := plugin.DefaultRegistry.Get(ctx, cwd, p.Skill)
	if err != nil {
		return "", err
	}
	if !entry.Invocation.IsModelInvocable() {
		return "", fmt.Errorf("skill %q is not invocable by the model (policy restricted)", p.Skill)
	}

	// Canonical rendering (port of renderSkillContent)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Skill: %s", entry.Name))
	if entry.Provider != "" {
		sb.WriteString(fmt.Sprintf(" (Provider: %s)", entry.Provider))
	}
	sb.WriteString("\n")
	if entry.ResourceBase != "" {
		sb.WriteString(fmt.Sprintf("Resource Base: %s\n", entry.ResourceBase))
	}
	if entry.Path != "" {
		sb.WriteString(fmt.Sprintf("Source: %s\n", entry.Path))
	}
	sb.WriteString("\n")
	sb.WriteString(entry.Content)
	return sb.String(), nil
}
