package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GrayCodeAI/hawk/internal/storage"
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

func (SkillTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Skill string `json:"skill"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
	}
	skills := discoverSkills()
	if p.Skill == "" {
		if len(skills) == 0 {
			return "No skills found in Hawk user state, .agents/skills, or .codex/skills.", nil
		}
		names := make([]string, 0, len(skills))
		for name := range skills {
			names = append(names, name)
		}
		sort.Strings(names)
		return "Available skills:\n" + strings.Join(names, "\n"), nil
	}
	path, ok := skills[p.Skill]
	if !ok {
		return "", fmt.Errorf("skill %q not found", p.Skill)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("# Skill: %s\nSource: %s\n\n%s", p.Skill, path, string(data)), nil
}

func discoverSkills() map[string]string {
	roots := skillRoots()
	out := make(map[string]string)
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(root, entry.Name())
			if entry.IsDir() {
				for _, filename := range []string{"SKILL.md", "skill.md", entry.Name() + ".md"} {
					candidate := filepath.Join(path, filename)
					if fileExists(candidate) {
						out[entry.Name()] = candidate
						break
					}
				}
				continue
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
				name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
				out[name] = path
			}
		}
	}
	return out
}

func skillRoots() []string {
	var roots []string
	roots = append(roots, filepath.Join(storage.StateDir(), "skills"))
	if cwd, err := os.Getwd(); err == nil {
		roots = append(
			roots,
			filepath.Join(cwd, ".agents", "skills"),
			filepath.Join(cwd, ".claude", "skills"),
			filepath.Join(cwd, ".codex", "skills"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(
			roots,
			filepath.Join(home, ".agents", "skills"),
			filepath.Join(home, ".claude", "skills"),
			filepath.Join(home, ".codex", "skills"),
		)
	}
	return roots
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
