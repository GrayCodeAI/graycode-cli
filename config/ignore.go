package config

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultIgnorePatterns returns the built-in ignore patterns.
func DefaultIgnorePatterns() []string {
	return []string{
		"node_modules",
		".git",
		"__pycache__",
		".venv",
		"dist",
		"build",
		"*.pyc",
		".DS_Store",
	}
}

// LoadIgnorePatterns reads ignore patterns from .hawkignore or .hawk/ignore.
// Falls back to default patterns if neither file exists.
func LoadIgnorePatterns() []string {
	// Try .hawkignore first, then .hawk/ignore
	for _, name := range []string{".hawkignore", filepath.Join(".hawk", "ignore")} {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		patterns := parseIgnoreFile(string(data))
		if len(patterns) > 0 {
			return patterns
		}
	}
	return DefaultIgnorePatterns()
}

// parseIgnoreFile parses a gitignore-style file into a list of patterns.
// Blank lines and comments (lines starting with #) are skipped.
func parseIgnoreFile(content string) []string {
	var patterns []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// ShouldIgnore checks if a path matches any of the given ignore patterns.
// Supports gitignore-style matching:
//   - Simple names match any path component (e.g., "node_modules" matches "a/node_modules/b")
//   - Glob patterns with * are matched against the base name (e.g., "*.pyc" matches "foo.pyc")
//   - Paths with / are matched against the full path
func ShouldIgnore(path string, patterns []string) bool {
	// Normalize path separators
	path = filepath.ToSlash(path)
	base := filepath.Base(path)

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}

		// If pattern contains a slash, match against the full path
		if strings.Contains(pattern, "/") {
			if matchGlob(pattern, path) {
				return true
			}
			continue
		}

		// Match against base name
		if matchGlob(pattern, base) {
			return true
		}

		// Also check if any path component matches the pattern exactly
		for _, component := range strings.Split(path, "/") {
			if matchGlob(pattern, component) {
				return true
			}
		}
	}
	return false
}

// matchGlob performs simple glob matching supporting * wildcards.
// * matches any sequence of non-separator characters.
func matchGlob(pattern, name string) bool {
	// Use filepath.Match for standard glob matching
	matched, err := filepath.Match(pattern, name)
	if err != nil {
		// If pattern is invalid, try exact match
		return pattern == name
	}
	return matched
}
