package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GrayCodeAI/hawk/internal/storage"
)

const (
	// toolOutputSpillMinChars — above this, tool output is written to disk (Cursor-style).
	toolOutputSpillMinChars = 12_000
	toolOutputSpillPreview  = 2_000
)

// maybeSpillToolOutput writes very large tool output to Hawk's user cache and returns a short handle.
func maybeSpillToolOutput(output, toolName, toolID string) string {
	if len(output) <= toolOutputSpillMinChars {
		return output
	}
	cwd, err := os.Getwd()
	if err != nil {
		return truncateToolOutput(output, toolOutputSpillMinChars)
	}
	dir := filepath.Join(storage.ProjectCacheDir(cwd), "scratch")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return truncateToolOutput(output, toolOutputSpillMinChars)
	}
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, toolName)
	if safe == "" {
		safe = "tool"
	}
	id := strings.TrimSpace(toolID)
	if id == "" {
		id = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if len(id) > 12 {
		id = id[:12]
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.txt", safe, id))
	if err := os.WriteFile(path, []byte(output), 0o600); err != nil {
		return truncateToolOutput(output, toolOutputSpillMinChars)
	}
	preview := output
	if len(preview) > toolOutputSpillPreview {
		preview = preview[:toolOutputSpillPreview] + "\n... (see file for full output)"
	}
	return fmt.Sprintf(
		"Output saved to %s (%d bytes).\n\nPreview:\n%s\n\nUse Read (offset/limit) or Grep on this path to inspect the rest.",
		path, len(output), preview,
	)
}

func truncateToolOutput(output string, max int) string {
	if len(output) <= max {
		return output
	}
	return output[:max] + "\n... (truncated)"
}
