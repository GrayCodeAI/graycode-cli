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

type GrepTool struct{}

func (GrepTool) Name() string        { return "Grep" }
func (GrepTool) RiskLevel() string   { return "low" }
func (GrepTool) Aliases() []string   { return []string{"grep"} }
func (GrepTool) Description() string { return "Search for a regex pattern in files." }
func (GrepTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{"type": "string", "description": "Regex pattern to search for"},
			"path":    map[string]interface{}{"type": "string", "description": "Directory to search (default: current dir)"},
			"include": map[string]interface{}{"type": "string", "description": "File glob filter (e.g. *.go)"},
		},
		"required": []string{"pattern"},
	}
}

func (GrepTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}
	root := p.Path
	if root == "" {
		root = "."
	}
	if err := validatePathAllowed(ctx, root); err != nil {
		return "", err
	}
	var results []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if p.Include != "" {
			if matched, _ := filepath.Match(p.Include, d.Name()); !matched {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d: %s", path, i+1, line))
				if len(results) >= 200 {
					return fmt.Errorf("limit")
				}
			}
		}
		return nil
	})
	if len(results) == 0 {
		return "No matches found", nil
	}
	return strings.Join(results, "\n"), nil
}
