package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkflowDef defines a scripted workflow loaded from .hawk/workflows/.
type WorkflowDef struct {
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	Steps       []WorkflowStep `yaml:"steps" json:"steps"`
}

// WorkflowStep is a single step in a workflow.
type WorkflowStep struct {
	Name    string `yaml:"name" json:"name"`
	Prompt  string `yaml:"prompt" json:"prompt"`
	Tool    string `yaml:"tool" json:"tool"`
	Input   string `yaml:"input" json:"input"`
	OnError string `yaml:"on_error" json:"on_error"`
}

// WorkflowTool executes scripted workflows from .hawk/workflows/.
type WorkflowTool struct{}

func (WorkflowTool) Name() string        { return "Workflow" }
func (WorkflowTool) Aliases() []string   { return []string{"workflow"} }
func (WorkflowTool) Description() string {
	return "Execute a scripted workflow defined in .hawk/workflows/"
}
func (WorkflowTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"workflow": map[string]interface{}{
				"type":        "string",
				"description": "Name of the workflow to execute (from .hawk/workflows/)",
			},
			"args": map[string]interface{}{
				"type":        "object",
				"description": "Arguments to pass to the workflow",
			},
		},
		"required": []string{"workflow"},
	}
}

func (WorkflowTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Workflow string         `json:"workflow"`
		Args    map[string]any `json:"args"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Workflow == "" {
		return "", fmt.Errorf("workflow name is required")
	}

	def, err := loadWorkflow(p.Workflow)
	if err != nil {
		return "", err
	}

	var results []map[string]any
	for i, step := range def.Steps {
		prompt := step.Prompt
		// Substitute args into prompt
		for k, v := range p.Args {
			prompt = strings.ReplaceAll(prompt, fmt.Sprintf("{{%s}}", k), fmt.Sprintf("%v", v))
		}
		results = append(results, map[string]any{
			"step":   i + 1,
			"name":   step.Name,
			"prompt": prompt,
			"tool":   step.Tool,
		})
	}

	out, _ := json.Marshal(map[string]any{
		"workflow":    def.Name,
		"description": def.Description,
		"steps":      results,
		"totalSteps": len(def.Steps),
	})
	return string(out), nil
}

func loadWorkflow(name string) (*WorkflowDef, error) {
	// Search in .hawk/workflows/ relative to cwd, then home
	searchPaths := []string{
		filepath.Join(".hawk", "workflows", name+".yml"),
		filepath.Join(".hawk", "workflows", name+".yaml"),
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		searchPaths = append(searchPaths,
			filepath.Join(home, ".hawk", "workflows", name+".yml"),
			filepath.Join(home, ".hawk", "workflows", name+".yaml"),
		)
	}

	for _, path := range searchPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var def WorkflowDef
		if err := yaml.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("parsing workflow %q: %w", path, err)
		}
		if def.Name == "" {
			def.Name = name
		}
		return &def, nil
	}
	return nil, fmt.Errorf("workflow %q not found in .hawk/workflows/", name)
}

// ListWorkflows discovers available workflows.
func ListWorkflows() []WorkflowDef {
	var workflows []WorkflowDef
	searchDirs := []string{filepath.Join(".hawk", "workflows")}
	home, _ := os.UserHomeDir()
	if home != "" {
		searchDirs = append(searchDirs, filepath.Join(home, ".hawk", "workflows"))
	}

	seen := make(map[string]bool)
	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := filepath.Ext(e.Name())
			if ext != ".yml" && ext != ".yaml" {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ext)
			if seen[name] {
				continue
			}
			seen[name] = true
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var def WorkflowDef
			if err := yaml.Unmarshal(data, &def); err != nil {
				continue
			}
			if def.Name == "" {
				def.Name = name
			}
			workflows = append(workflows, def)
		}
	}
	return workflows
}
