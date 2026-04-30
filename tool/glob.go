package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type GlobTool struct{}

func (GlobTool) Name() string        { return "glob" }
func (GlobTool) Description() string { return "Find files matching a glob pattern." }
func (GlobTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pattern": map[string]interface{}{"type": "string", "description": "Glob pattern (e.g. **/*.go)"},
			"path":    map[string]interface{}{"type": "string", "description": "Root directory (default: current dir)"},
		},
		"required": []string{"pattern"},
	}
}

func (GlobTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	root := p.Path
	if root == "" {
		root = "."
	}
	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "dist") {
			return filepath.SkipDir
		}
		matched, _ := filepath.Match(p.Pattern, filepath.Base(path))
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "No files found", nil
	}
	return fmt.Sprintf("%d files:\n%s", len(matches), strings.Join(matches, "\n")), nil
}
