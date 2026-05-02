package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type LSTool struct{}

func (LSTool) Name() string      { return "LS" }
func (LSTool) RiskLevel() string { return "low" }
func (LSTool) Aliases() []string { return []string{"ls"} }
func (LSTool) Description() string {
	return "List files and directories in a directory."
}
func (LSTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":   map[string]interface{}{"type": "string", "description": "Directory path to list (default: current directory)"},
			"ignore": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Optional file or glob patterns to exclude"},
		},
	}
}

func (LSTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path   string   `json:"path"`
		Ignore []string `json:"ignore"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &p); err != nil {
			return "", err
		}
	}
	path := p.Path
	if path == "" {
		path = "."
	}
	if err := validatePathAllowed(ctx, path); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("ls %s: %w", path, err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	var lines []string
	for _, entry := range entries {
		name := entry.Name()
		fullPath := filepath.Join(path, name)
		if ignoredByLS(name, fullPath, p.Ignore) {
			continue
		}
		if entry.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	if len(lines) == 0 {
		return fmt.Sprintf("%s: no entries", path), nil
	}
	return fmt.Sprintf("%s:\n%s", path, strings.Join(lines, "\n")), nil
}

func ignoredByLS(name, path string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if pattern == name || pattern == path {
			return true
		}
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}
