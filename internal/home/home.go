package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir returns the user's home directory.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return home, nil
}

// MustDir returns the user's home directory, panicking if it cannot be
// determined. Use only in init-time code where recovery is impossible.
func MustDir() string {
	home, err := Dir()
	if err != nil {
		panic(err)
	}
	return home
}

// Expand expands a leading "~", "~/" or "$HOME" in path to the user's
// home directory. Paths without such a prefix are returned unchanged.
func Expand(path string) (string, error) {
	if path == "~" || path == "$HOME" {
		return Dir()
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(filepath.Separator)) {
		home, err := Dir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	if strings.HasPrefix(path, "$HOME") {
		home, err := Dir()
		if err != nil {
			return "", err
		}
		return home + strings.TrimPrefix(path, "$HOME"), nil
	}
	return path, nil
}

// MustExpand is like Expand but panics on error. Use only in init-time code.
func MustExpand(path string) string {
	expanded, err := Expand(path)
	if err != nil {
		panic(err)
	}
	return expanded
}
