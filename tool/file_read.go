package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxFileSize = 1 << 30 // 1 GiB

type FileReadTool struct{}

func (FileReadTool) Name() string        { return "file_read" }
func (FileReadTool) Description() string { return "Read a file's contents, optionally a specific line range." }
func (FileReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":       map[string]interface{}{"type": "string", "description": "File path to read"},
			"start_line": map[string]interface{}{"type": "integer", "description": "Start line (1-based, optional)"},
			"end_line":   map[string]interface{}{"type": "integer", "description": "End line (1-based, inclusive, optional)"},
		},
		"required": []string{"path"},
	}
}

func (FileReadTool) Execute(_ context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	info, err := os.Stat(p.Path)
	if err != nil {
		suggestion := suggestSimilar(p.Path)
		if suggestion != "" {
			return "", fmt.Errorf("file not found: %s\nDid you mean: %s", p.Path, suggestion)
		}
		return "", fmt.Errorf("file not found: %s", p.Path)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", p.Path, err)
	}
	if p.StartLine == 0 && p.EndLine == 0 {
		return string(data), nil
	}
	lines := strings.Split(string(data), "\n")
	start := max(1, p.StartLine) - 1
	end := len(lines)
	if p.EndLine > 0 {
		end = min(p.EndLine, len(lines))
	}
	if start >= len(lines) {
		return "", fmt.Errorf("start_line %d exceeds file length %d", p.StartLine, len(lines))
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%4d | %s\n", i+1, lines[i])
	}
	return b.String(), nil
}

// suggestSimilar finds a similar file in the same directory.
func suggestSimilar(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	best := ""
	bestScore := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		score := commonPrefix(strings.ToLower(base), strings.ToLower(e.Name()))
		if score > bestScore && score >= 3 {
			bestScore = score
			best = filepath.Join(dir, e.Name())
		}
	}
	return best
}

func commonPrefix(a, b string) int {
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}
