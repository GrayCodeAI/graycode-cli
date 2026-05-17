package repomap

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// IndexPatterns controls which files are indexed.
type IndexPatterns struct {
	Include []string `json:"include"` // if non-empty, ONLY files matching these are indexed
	Exclude []string `json:"exclude"` // files matching these are NEVER indexed (overrides include)
}

// DefaultIndexPatterns returns sensible defaults.
func DefaultIndexPatterns() IndexPatterns {
	return IndexPatterns{
		Include: []string{}, // empty = all supported files
		Exclude: []string{
			"vendor/**", "node_modules/**", ".git/**", "dist/**", "build/**",
			"__pycache__/**", ".venv/**", "*.min.js", "*.min.css",
			"*.generated.*", "*.pb.go", "*_test.go", // skip test files from index
			"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		},
	}
}

// ShouldIndex checks if a path should be indexed based on include/exclude patterns.
func (p IndexPatterns) ShouldIndex(path string) bool {
	// Check exclude first (always wins)
	for _, pattern := range p.Exclude {
		if matched, _ := filepath.Match(pattern, path); matched {
			return false
		}
		// Also check against just the filename
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return false
		}
	}
	// If include is empty, allow everything
	if len(p.Include) == 0 {
		return true
	}
	// Check include patterns
	for _, pattern := range p.Include {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

// LoadIndexPatterns reads from .hawk/index.json or uses defaults.
func LoadIndexPatterns() IndexPatterns {
	configPath := filepath.Join(".hawk", "index.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return DefaultIndexPatterns()
	}
	var patterns IndexPatterns
	if err := json.Unmarshal(data, &patterns); err != nil {
		return DefaultIndexPatterns()
	}
	// Merge with defaults: if exclude is empty, use defaults
	if len(patterns.Exclude) == 0 {
		patterns.Exclude = DefaultIndexPatterns().Exclude
	}
	return patterns
}
