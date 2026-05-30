package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// sidecarPath returns the path to the migration sidecar file.
// For ~/.hawk/settings.json it returns ~/.hawk/settings.migrations.json.
func sidecarPath(configPath string) string {
	ext := ".json"
	base := configPath
	if idx := len(configPath) - len(ext); idx >= 0 && configPath[idx:] == ext {
		base = configPath[:idx]
	}
	return base + ".migrations.json"
}

// sidecarData is the JSON structure of the migration sidecar file.
type sidecarData struct {
	Applied []string `json:"applied_migrations"`
}

// AppliedMigrations reads the set of applied migration keys from the sidecar
// file adjacent to the given config path. Returns an empty set if the
// sidecar doesn't exist or can't be parsed.
func AppliedMigrations(configPath string) map[string]bool {
	data, err := os.ReadFile(sidecarPath(configPath))
	if err != nil {
		return make(map[string]bool)
	}
	var sd sidecarData
	if err := json.Unmarshal(data, &sd); err != nil {
		return make(map[string]bool)
	}
	result := make(map[string]bool, len(sd.Applied))
	for _, k := range sd.Applied {
		result[k] = true
	}
	return result
}

// RecordAppliedMigration adds a migration key to the sidecar file.
// Idempotent — recording the same key twice is a no-op.
func RecordAppliedMigration(configPath, migrationKey string) error {
	applied := AppliedMigrations(configPath)
	if applied[migrationKey] {
		return nil
	}
	keys := make([]string, 0, len(applied)+1)
	for k := range applied {
		keys = append(keys, k)
	}
	keys = append(keys, migrationKey)
	sort.Strings(keys)

	data, err := json.MarshalIndent(sidecarData{Applied: keys}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sidecarPath(configPath), data, 0o644)
}

// MigrationKey returns a unique key for a migration step.
func MigrationKey(m Migration) string {
	return fmt.Sprintf("v%d->v%d:%s", m.FromVersion, m.ToVersion, m.Description)
}
