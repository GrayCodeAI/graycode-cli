package engine

import (
	"os"
	"path/filepath"
	"strings"
)

// HintsFilenames are the files hawk auto-loads for project context.
var HintsFilenames = []string{".hawkhints", "AGENTS.md"}

// HintsLoader discovers and loads project-specific hint files from the
// working directory and subdirectories the agent explores.
type HintsLoader struct {
	loaded map[string]bool // tracks which dirs have been scanned
}

// NewHintsLoader creates a hints loader.
func NewHintsLoader() *HintsLoader {
	return &HintsLoader{loaded: make(map[string]bool)}
}

// LoadHints scans a directory for hint files and returns their combined content.
// Tracks which directories have been loaded to avoid re-reading.
func (h *HintsLoader) LoadHints(dir string) string {
	dir, _ = filepath.Abs(dir)
	if h.loaded[dir] {
		return ""
	}
	h.loaded[dir] = true

	var hints []string
	for _, name := range HintsFilenames {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			hints = append(hints, "# ["+name+" from "+filepath.Base(dir)+"]\n"+content)
		}
	}
	return strings.Join(hints, "\n\n")
}

// LoadHintsRecursive loads hints from dir and all parent dirs up to root.
func (h *HintsLoader) LoadHintsRecursive(dir string) string {
	dir, _ = filepath.Abs(dir)
	var all []string

	// Walk up to root collecting hints
	for {
		if hint := h.LoadHints(dir); hint != "" {
			all = append([]string{hint}, all...) // prepend (root first)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return strings.Join(all, "\n\n")
}

// IsLoaded reports whether a directory has already been scanned.
func (h *HintsLoader) IsLoaded(dir string) bool {
	dir, _ = filepath.Abs(dir)
	return h.loaded[dir]
}

// Reset clears the loaded state (e.g., on /new session).
func (h *HintsLoader) Reset() {
	h.loaded = make(map[string]bool)
}
