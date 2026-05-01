package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/GrayCodeAI/hawk/tool"
)

// filePathCompletions returns matching files/dirs from the partial path typed.
func filePathCompletions(partial string) []string {
	if partial == "" {
		partial = "."
	}

	dir := filepath.Dir(partial)
	base := filepath.Base(partial)

	// If partial ends with a separator, list that directory
	if strings.HasSuffix(partial, string(filepath.Separator)) || partial == "." {
		dir = partial
		base = ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var matches []string
	for _, e := range entries {
		name := e.Name()
		// Skip hidden files unless the user is explicitly typing a dot prefix
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if base == "" || strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			full := filepath.Join(dir, name)
			if e.IsDir() {
				full += string(filepath.Separator)
			}
			matches = append(matches, full)
		}
	}

	// Cap results to avoid flooding the UI
	if len(matches) > 20 {
		matches = matches[:20]
	}
	return matches
}

// toolNameCompletions returns matching tool names from the partial string typed.
func toolNameCompletions(partial string, registry *tool.Registry) []string {
	if registry == nil {
		return nil
	}

	partial = strings.ToLower(strings.TrimSpace(partial))
	if partial == "" {
		return nil
	}

	var matches []string
	for _, t := range registry.PrimaryTools() {
		name := t.Name()
		if strings.HasPrefix(strings.ToLower(name), partial) {
			matches = append(matches, name)
		}
	}

	if len(matches) > 20 {
		matches = matches[:20]
	}
	return matches
}
