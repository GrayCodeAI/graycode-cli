package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DefaultAliases returns the built-in command aliases.
func DefaultAliases() map[string]string {
	return map[string]string{
		"fix":    "Find and fix the bug in",
		"test":   "Write tests for",
		"review": "Review this code for issues:",
	}
}

// aliasesFilePath returns the path to the aliases config file.
func aliasesFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hawk", "aliases.json")
}

// LoadAliases reads command aliases from ~/.hawk/aliases.json.
// Returns default aliases if the file does not exist.
func LoadAliases() map[string]string {
	data, err := os.ReadFile(aliasesFilePath())
	if err != nil {
		return DefaultAliases()
	}
	var aliases map[string]string
	if err := json.Unmarshal(data, &aliases); err != nil {
		return DefaultAliases()
	}
	// Merge defaults for any missing keys
	defaults := DefaultAliases()
	for k, v := range defaults {
		if _, ok := aliases[k]; !ok {
			aliases[k] = v
		}
	}
	return aliases
}

// SaveAliases writes command aliases to ~/.hawk/aliases.json.
func SaveAliases(aliases map[string]string) error {
	path := aliasesFilePath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	data, err := json.MarshalIndent(aliases, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ResolveAlias checks if the first word of input matches an alias key.
// If found, it replaces the alias with its expansion and appends the rest of the input.
// If not found, returns the input unchanged.
func ResolveAlias(input string, aliases map[string]string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return input
	}

	// Split into first word and the rest
	parts := strings.SplitN(input, " ", 2)
	key := parts[0]

	expansion, ok := aliases[key]
	if !ok {
		return input
	}

	if len(parts) > 1 {
		return expansion + " " + parts[1]
	}
	return expansion
}
