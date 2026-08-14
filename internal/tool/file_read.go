package tool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxFileSize = 10 << 20 // 10 MiB

type FileReadTool struct{}

func (FileReadTool) Name() string      { return "Read" }
func (FileReadTool) RiskLevel() string { return "low" }
func (FileReadTool) Aliases() []string { return []string{"file_read"} }
func (FileReadTool) Description() string {
	return "Read a file's contents, optionally a specific line range."
}

// Schema returns the typed input schema for FileRead. Parameters() derives
// from it so the wire format and validator never diverge.
func (FileReadTool) Schema() ToolSchema {
	return ToolSchema{
		Type: "object",
		Properties: map[string]SchemaProperty{
			"path":       {Type: "string", Description: "File path to read"},
			"file_path":  {Type: "string", Description: "Archive-compatible alias for path"},
			"start_line": {Type: "integer", Description: "Start line (1-based, optional)"},
			"end_line":   {Type: "integer", Description: "End line (1-based, inclusive, optional)"},
			"offset":     {Type: "integer", Description: "Archive-compatible 1-based start line alias"},
			"limit":      {Type: "integer", Description: "Archive-compatible number of lines to read"},
		},
		Required: []string{"path"},
	}
}

func (FileReadTool) Parameters() map[string]interface{} {
	return fileReadSchema.ToJSONSchema()
}

// fileReadSchema is the single source of truth for FileRead's input schema.
var fileReadSchema = FileReadTool{}.Schema()

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
	// Check the caller's path first (block sensitive paths even when the
	// file is missing), then resolve symlinks so the checks apply to the
	// real target (M13): a symlink to ~/.ssh/id_rsa must be caught here,
	// not silently followed during the read.
	if err := validatePathAllowed(ctx, path); err != nil {
		return "", err
	}
	if reason := IsSensitivePath(path); reason != "" {
		return "", fmt.Errorf("blocked: %s", reason)
	}
	resolved := path
	if canonical, err := filepath.EvalSymlinks(path); err == nil {
		resolved = canonical
	} else {
		// Nonexistent file or dangling symlink — report as not found.
		suggestion := suggestSimilar(path)
		if suggestion != "" {
			return "", fmt.Errorf("file not found: %s\nDid you mean: %s", path, suggestion)
		}
		return "", fmt.Errorf("file not found: %s", path)
	}
	if err := validatePathAllowed(ctx, resolved); err != nil {
		return "", err
	}
	if reason := IsSensitivePath(resolved); reason != "" {
		return "", fmt.Errorf("blocked: %s", reason)
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

	f, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	// TOCTOU guard: the fd is fixed, but verify it is the exact file we
	// validated — if a symlink was swapped in after resolution, Lstat sees
	// the symlink and SameFile fails (M13).
	if li, lerr := os.Lstat(resolved); lerr == nil {
		if !os.SameFile(info, li) {
			return "", fmt.Errorf("read %s: file changed during access (symlink swap rejected)", path)
		}
	}
	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), maxFileSize)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if IsBinaryContent(data) {
		// Multi-modal vision: encode images as base64 data URIs
		if isImageFile(resolved) {
			ext := strings.ToLower(filepath.Ext(resolved))
			mimeType := imageExtensions[ext]
			if mimeType == "" {
				mimeType = "image/png"
			}
			encoded := base64.StdEncoding.EncodeToString(data)
			dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
			return fmt.Sprintf("[IMAGE: %s]\n%s", filepath.Base(resolved), dataURI), nil
		}
		return BinaryIndicator, nil
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
		_, _ = fmt.Fprintf(&b, "%4d | %s\n", i+1, lines[i])
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
