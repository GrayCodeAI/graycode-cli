package home

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Dir returns the user's home directory. It calls os.Exit(1) if the
// home directory cannot be determined. Use this for critical paths where an
// empty home would cause data to be written to the wrong location.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("cannot determine home directory", "error", err)
		os.Exit(1)
	}
	return home
}

// Expand expands a leading "~", "~/" or "$HOME" in path to the user's
// home directory. Paths without such a prefix are returned unchanged.
// Like Dir, it exits if the home directory cannot be determined.
func Expand(path string) string {
	if path == "~" || path == "$HOME" {
		return Dir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		return filepath.Join(Dir(), path[2:])
	}
	if strings.HasPrefix(path, "$HOME") {
		return Dir() + strings.TrimPrefix(path, "$HOME")
	}
	return path
}
