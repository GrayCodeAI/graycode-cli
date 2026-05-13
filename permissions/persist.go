package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Rule is defined in rules.go with the Action type.

// Save serializes permission rules to a JSON file atomically.
func Save(path string, rules []Rule) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create permissions directory: %w", err)
	}

	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal permissions: %w", err)
	}

	// Atomic write: write to temp file then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write permissions temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename permissions file: %w", err)
	}
	return nil
}

// Load deserializes permission rules from a JSON file.
func Load(path string) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no file, no rules
		}
		return nil, fmt.Errorf("read permissions file: %w", err)
	}

	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("unmarshal permissions: %w", err)
	}
	return rules, nil
}

// DefaultPath returns the default permissions file path for a project directory.
func DefaultPath(projectDir string) string {
	return filepath.Join(projectDir, ".hawk", "permissions.json")
}
