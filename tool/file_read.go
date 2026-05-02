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

func (FileReadTool) Name() string      { return "Read" }
func (FileReadTool) Aliases() []string { return []string{"file_read"} }
func (FileReadTool) Description() string {
	return "Read a file's contents, optionally a specific line range."
}
func (FileReadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path":       map[string]interface{}{"type": "string", "description": "File path to read"},
			"file_path":  map[string]interface{}{"type": "string", "description": "Archive-compatible alias for path"},
			"start_line": map[string]interface{}{"type": "integer", "description": "Start line (1-based, optional)"},
			"end_line":   map[string]interface{}{"type": "integer", "description": "End line (1-based, inclusive, optional)"},
			"offset":     map[string]interface{}{"type": "integer", "description": "Archive-compatible 1-based start line alias"},
			"limit":      map[string]interface{}{"type": "integer", "description": "Archive-compatible number of lines to read"},
		},
	}
}

func (FileReadTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Path      string `json:"path"`
		FilePath  string `json:"file_path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
		Offset    int    `json:"offset"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	path := p.Path
	if path == "" {
		path = p.FilePath
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := validatePathAllowed(ctx, path); err != nil {
		return "", err
	}
	startLine, endLine := p.StartLine, p.EndLine
	if p.Offset > 0 {
		startLine = p.Offset
		if p.Limit > 0 {
			endLine = p.Offset + p.Limit - 1
		}
	} else if startLine == 0 && p.Limit > 0 {
		startLine = 1
		endLine = p.Limit
	}

	info, err := os.Stat(path)
	if err != nil {
		suggestion := suggestSimilar(path)
		if suggestion != "" {
			return "", fmt.Errorf("file not found: %s\nDid you mean: %s", path, suggestion)
		}
		return "", fmt.Errorf("file not found: %s", path)
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	data = StripBOM(data)
	if startLine == 0 && endLine == 0 {
		return string(data), nil
	}
	lines := strings.Split(string(data), "\n")
	start := max(1, startLine) - 1
	end := len(lines)
	if endLine > 0 {
		end = min(endLine, len(lines))
	}
	if start >= len(lines) {
		return "", fmt.Errorf("start_line %d exceeds file length %d", startLine, len(lines))
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
