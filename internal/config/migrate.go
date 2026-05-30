package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Migration represents a single config version upgrade step.
type Migration struct {
	FromVersion int
	ToVersion   int
	Description string
	Migrate     func(data map[string]interface{}) (map[string]interface{}, error)
}

// MigrationRegistry holds all registered migrations and the current target version.
type MigrationRegistry struct {
	Migrations     []Migration
	CurrentVersion int
}

// NewMigrationRegistry returns a registry with all default migrations registered.
func NewMigrationRegistry() *MigrationRegistry {
	r := &MigrationRegistry{
		CurrentVersion: 8,
	}

	// v1→v2: Rename apiKey to api_key (snake_case normalization)
	r.Migrations = append(r.Migrations, Migration{
		FromVersion: 1,
		ToVersion:   2,
		Description: "Rename apiKey to api_key (snake_case normalization)",
		Migrate: func(data map[string]interface{}) (map[string]interface{}, error) {
			if val, ok := data["apiKey"]; ok {
				data["api_key"] = val
				delete(data, "apiKey")
			}
			return data, nil
		},
	})

	// v2→v3: Move provider.model to top-level model
	r.Migrations = append(r.Migrations, Migration{
		FromVersion: 2,
		ToVersion:   3,
		Description: "Move provider.model to top-level model",
		Migrate: func(data map[string]interface{}) (map[string]interface{}, error) {
			if provider, ok := data["provider"].(map[string]interface{}); ok {
				if model, ok := provider["model"]; ok {
					data["model"] = model
					delete(provider, "model")
					// If provider object is now empty, remove it; otherwise keep remaining fields
					if len(provider) == 0 {
						delete(data, "provider")
					}
				}
			}
			return data, nil
		},
	})

	// v3→v4: Add permissions section with defaults
	r.Migrations = append(r.Migrations, Migration{
		FromVersion: 3,
		ToVersion:   4,
		Description: "Add permissions section with defaults",
		Migrate: func(data map[string]interface{}) (map[string]interface{}, error) {
			if _, ok := data["permissions"]; !ok {
				data["permissions"] = map[string]interface{}{
					"allow_read":    true,
					"allow_write":   true,
					"allow_execute": false,
					"allow_network": false,
				}
			}
			return data, nil
		},
	})

	// v4→v5: Rename autoMode to auto_approve, add guardian section
	r.Migrations = append(r.Migrations, Migration{
		FromVersion: 4,
		ToVersion:   5,
		Description: "Rename autoMode to auto_approve, add guardian section",
		Migrate: func(data map[string]interface{}) (map[string]interface{}, error) {
			if val, ok := data["autoMode"]; ok {
				data["auto_approve"] = val
				delete(data, "autoMode")
			}
			if _, ok := data["guardian"]; !ok {
				data["guardian"] = map[string]interface{}{
					"enabled":    true,
					"max_risk":   "medium",
					"block_list": []interface{}{},
				}
			}
			return data, nil
		},
	})

	// v5→v6: Add tok compression settings with defaults
	r.Migrations = append(r.Migrations, Migration{
		FromVersion: 5,
		ToVersion:   6,
		Description: "Add tok compression settings with defaults",
		Migrate: func(data map[string]interface{}) (map[string]interface{}, error) {
			if _, ok := data["tok"]; !ok {
				data["tok"] = map[string]interface{}{
					"compression_mode": "full",
					"max_tokens":       100000,
					"preserve_code":    true,
				}
			}
			return data, nil
		},
	})

	// v6→v7: Add cache_warming and cost_limits sections
	r.Migrations = append(r.Migrations, Migration{
		FromVersion: 6,
		ToVersion:   7,
		Description: "Add cache_warming and cost_limits sections",
		Migrate: func(data map[string]interface{}) (map[string]interface{}, error) {
			if _, ok := data["cache_warming"]; !ok {
				data["cache_warming"] = map[string]interface{}{
					"enabled":    false,
					"strategy":   "recent_files",
					"max_files":  10,
					"on_startup": false,
				}
			}
			if _, ok := data["cost_limits"]; !ok {
				data["cost_limits"] = map[string]interface{}{
					"max_per_session": 5.0,
					"max_per_day":     25.0,
					"warn_threshold":  0.8,
					"hard_stop":       true,
				}
			}
			return data, nil
		},
	})

	// v7→v8: Restructure sandbox from bool to object {enabled, type, network}
	r.Migrations = append(r.Migrations, Migration{
		FromVersion: 7,
		ToVersion:   8,
		Description: "Restructure sandbox from bool to object {enabled, type, network}",
		Migrate: func(data map[string]interface{}) (map[string]interface{}, error) {
			sandboxVal, exists := data["sandbox"]
			if !exists {
				data["sandbox"] = map[string]interface{}{
					"enabled": false,
					"type":    "landlock",
					"network": false,
				}
				return data, nil
			}
			switch v := sandboxVal.(type) {
			case bool:
				data["sandbox"] = map[string]interface{}{
					"enabled": v,
					"type":    "landlock",
					"network": false,
				}
			case string:
				enabled := v != "" && v != "off" && v != "false"
				sandboxType := "landlock"
				if v == "docker" || v == "chroot" {
					sandboxType = v
				}
				data["sandbox"] = map[string]interface{}{
					"enabled": enabled,
					"type":    sandboxType,
					"network": false,
				}
			case map[string]interface{}:
				// Already an object, ensure all fields present
				if _, ok := v["enabled"]; !ok {
					v["enabled"] = false
				}
				if _, ok := v["type"]; !ok {
					v["type"] = "landlock"
				}
				if _, ok := v["network"]; !ok {
					v["network"] = false
				}
				data["sandbox"] = v
			default:
				data["sandbox"] = map[string]interface{}{
					"enabled": false,
					"type":    "landlock",
					"network": false,
				}
			}
			return data, nil
		},
	})

	return r
}

// NeedsMigration returns true if the config data is at a version below CurrentVersion.
func (r *MigrationRegistry) NeedsMigration(data map[string]interface{}) bool {
	version := r.getVersion(data)
	return version < r.CurrentVersion
}

// Run applies all relevant migrations sequentially from the config's current version
// to the registry's CurrentVersion. Each migration is atomic: if one fails, we return
// the error without applying further migrations.
func (r *MigrationRegistry) Run(data map[string]interface{}) (map[string]interface{}, error) {
	version := r.getVersion(data)
	if version >= r.CurrentVersion {
		return data, nil
	}

	// Sort migrations by FromVersion
	sorted := make([]Migration, len(r.Migrations))
	copy(sorted, r.Migrations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].FromVersion < sorted[j].FromVersion
	})

	for _, m := range sorted {
		if m.FromVersion >= version && m.ToVersion <= r.CurrentVersion {
			if m.FromVersion != version {
				continue
			}
			// Make a shallow copy for atomicity check
			backup := make(map[string]interface{}, len(data))
			for k, v := range data {
				backup[k] = v
			}

			result, err := m.Migrate(data)
			if err != nil {
				// Restore from backup on failure (atomic)
				for k := range data {
					delete(data, k)
				}
				for k, v := range backup {
					data[k] = v
				}
				return data, fmt.Errorf("migration v%d→v%d failed: %w", m.FromVersion, m.ToVersion, err)
			}
			data = result
			data["config_version"] = m.ToVersion
			version = m.ToVersion
		}
	}

	return data, nil
}

// RunWithSidecar is like Run but tracks applied migrations in a sidecar file
// adjacent to configPath. Already-applied migrations are skipped.
func (r *MigrationRegistry) RunWithSidecar(data map[string]interface{}, configPath string) (map[string]interface{}, error) {
	version := r.getVersion(data)
	if version >= r.CurrentVersion {
		return data, nil
	}

	sorted := make([]Migration, len(r.Migrations))
	copy(sorted, r.Migrations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].FromVersion < sorted[j].FromVersion
	})

	applied := AppliedMigrations(configPath)

	for _, m := range sorted {
		if m.FromVersion >= version && m.ToVersion <= r.CurrentVersion {
			if m.FromVersion != version {
				continue
			}

			key := MigrationKey(m)
			if applied[key] {
				// Already applied — update version and skip
				data["config_version"] = m.ToVersion
				version = m.ToVersion
				continue
			}

			backup := make(map[string]interface{}, len(data))
			for k, v := range data {
				backup[k] = v
			}

			result, err := m.Migrate(data)
			if err != nil {
				for k := range data {
					delete(data, k)
				}
				for k, v := range backup {
					data[k] = v
				}
				return data, fmt.Errorf("migration v%d→v%d failed: %w", m.FromVersion, m.ToVersion, err)
			}
			data = result
			data["config_version"] = m.ToVersion
			version = m.ToVersion

			if recordErr := RecordAppliedMigration(configPath, key); recordErr != nil {
				// Non-fatal: log but continue
				fmt.Fprintf(os.Stderr, "warning: failed to record migration %s: %v\n", key, recordErr)
			}
		}
	}

	return data, nil
}

// Backup creates a timestamped backup of the config file before migration.
// Returns the path to the backup file.
func (r *MigrationRegistry) Backup(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config for backup: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := configPath + ".bak." + timestamp

	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}

// MigrateFile reads a config JSON file, checks if migration is needed,
// creates a backup, applies migrations, and writes the updated config.
func (r *MigrationRegistry) MigrateFile(path string) error {
	rawData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(rawData, &data); err != nil {
		return fmt.Errorf("failed to parse config JSON: %w", err)
	}

	if !r.NeedsMigration(data) {
		return nil
	}

	// Create backup before migration
	_, err = r.Backup(path)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	// Apply migrations
	result, err := r.Run(data)
	if err != nil {
		return err
	}

	// Write updated config
	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal migrated config: %w", err)
	}

	if err := os.WriteFile(path, output, 0o644); err != nil {
		return fmt.Errorf("failed to write migrated config: %w", err)
	}

	return nil
}

// ValidateConfig checks that a migrated config has required fields and correct types.
// Returns a list of warnings (empty list means valid).
func ValidateConfig(data map[string]interface{}) []string {
	var warnings []string

	// Check for required fields at current version (v8)
	requiredSections := []string{"permissions", "guardian", "tok", "cache_warming", "cost_limits", "sandbox"}
	for _, section := range requiredSections {
		if _, ok := data[section]; !ok {
			warnings = append(warnings, fmt.Sprintf("missing required section: %s", section))
		}
	}

	// Check for deprecated fields
	deprecatedFields := map[string]string{
		"apiKey":   "use api_key instead",
		"autoMode": "use auto_approve instead",
	}
	for field, msg := range deprecatedFields {
		if _, ok := data[field]; ok {
			warnings = append(warnings, fmt.Sprintf("deprecated field %q found: %s", field, msg))
		}
	}

	// Type checks for known sections
	if sandbox, ok := data["sandbox"]; ok {
		if _, isMap := sandbox.(map[string]interface{}); !isMap {
			warnings = append(warnings, "sandbox should be an object with {enabled, type, network}")
		}
	}

	if permissions, ok := data["permissions"]; ok {
		if _, isMap := permissions.(map[string]interface{}); !isMap {
			warnings = append(warnings, "permissions should be an object")
		}
	}

	if guardian, ok := data["guardian"]; ok {
		if _, isMap := guardian.(map[string]interface{}); !isMap {
			warnings = append(warnings, "guardian should be an object")
		}
	}

	if tok, ok := data["tok"]; ok {
		if _, isMap := tok.(map[string]interface{}); !isMap {
			warnings = append(warnings, "tok should be an object")
		}
	}

	if costLimits, ok := data["cost_limits"]; ok {
		if clMap, isMap := costLimits.(map[string]interface{}); isMap {
			if maxSession, ok := clMap["max_per_session"]; ok {
				if _, isNum := maxSession.(float64); !isNum {
					warnings = append(warnings, "cost_limits.max_per_session should be a number")
				}
			}
		} else {
			warnings = append(warnings, "cost_limits should be an object")
		}
	}

	if cacheWarming, ok := data["cache_warming"]; ok {
		if _, isMap := cacheWarming.(map[string]interface{}); !isMap {
			warnings = append(warnings, "cache_warming should be an object")
		}
	}

	// Check config_version type
	if ver, ok := data["config_version"]; ok {
		switch ver.(type) {
		case float64, int:
			// ok
		default:
			warnings = append(warnings, "config_version should be an integer")
		}
	}

	sort.Strings(warnings)
	return warnings
}

// DiffConfigs returns a human-readable diff showing what changed during migration.
func DiffConfigs(old, new map[string]interface{}) string {
	var lines []string

	oldVersion := getVersionFromMap(old)
	newVersion := getVersionFromMap(new)

	lines = append(lines, fmt.Sprintf("Config Migration v%d → v%d:", oldVersion, newVersion))

	// Find added keys
	for key := range new {
		if key == "config_version" {
			continue
		}
		if _, exists := old[key]; !exists {
			val := formatValue(new[key])
			lines = append(lines, fmt.Sprintf("  + Added: %s = %s", key, val))
		}
	}

	// Find removed keys
	for key := range old {
		if key == "config_version" {
			continue
		}
		if _, exists := new[key]; !exists {
			lines = append(lines, fmt.Sprintf("  - Removed: %s", key))
		}
	}

	// Find renamed fields (heuristic: same value, old key gone, new key appeared)
	renames := detectRenames(old, new)
	for _, rename := range renames {
		lines = append(lines, fmt.Sprintf("  ~ Renamed: %s → %s", rename[0], rename[1]))
	}

	// Find moved fields
	moves := detectMoves(old, new)
	for _, move := range moves {
		lines = append(lines, fmt.Sprintf("  ~ Moved: %s → %s", move[0], move[1]))
	}

	// Find changed keys (present in both but different)
	for key := range new {
		if key == "config_version" {
			continue
		}
		oldVal, existsInOld := old[key]
		if !existsInOld {
			continue
		}
		newVal := new[key]
		oldJSON, _ := json.Marshal(oldVal)
		newJSON, _ := json.Marshal(newVal)
		if string(oldJSON) != string(newJSON) {
			lines = append(lines, fmt.Sprintf("  ~ Changed: %s", key))
		}
	}

	if len(lines) == 1 {
		lines = append(lines, "  (no changes)")
	}

	return strings.Join(lines, "\n")
}

// RollbackMigration restores a config file from its backup.
func RollbackMigration(configPath, backupPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to restore config from backup: %w", err)
	}

	return nil
}

// DetectVersion uses heuristics to determine the config version when no
// config_version field is present.
func DetectVersion(data map[string]interface{}) int {
	// If config_version is explicitly set, use it
	if ver, ok := data["config_version"]; ok {
		switch v := ver.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}

	// Heuristic detection in reverse order (highest version first)

	// Has sandbox as object → v8
	if sandbox, ok := data["sandbox"]; ok {
		if _, isMap := sandbox.(map[string]interface{}); isMap {
			return 8
		}
	}

	// Has cache_warming or cost_limits → v7
	if _, ok := data["cache_warming"]; ok {
		return 7
	}
	if _, ok := data["cost_limits"]; ok {
		return 7
	}

	// Has tok section → v6
	if _, ok := data["tok"]; ok {
		return 6
	}

	// Has guardian section or auto_approve → v5
	if _, ok := data["guardian"]; ok {
		return 5
	}
	if _, ok := data["auto_approve"]; ok {
		return 5
	}

	// Has permissions section → v4
	if _, ok := data["permissions"]; ok {
		return 4
	}

	// Has top-level model (not nested in provider) → v3
	if _, ok := data["model"]; ok {
		if provider, ok := data["provider"].(map[string]interface{}); ok {
			if _, hasModel := provider["model"]; !hasModel {
				return 3
			}
		} else {
			return 3
		}
	}

	// Has provider.model nested → v2
	if provider, ok := data["provider"].(map[string]interface{}); ok {
		if _, hasModel := provider["model"]; hasModel {
			return 2
		}
	}

	// Has api_key (snake_case) → v2
	if _, ok := data["api_key"]; ok {
		return 2
	}

	// Has apiKey (camelCase) → v1
	if _, ok := data["apiKey"]; ok {
		return 1
	}

	// Default: assume v1
	return 1
}

// ─────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────

func (r *MigrationRegistry) getVersion(data map[string]interface{}) int {
	if ver, ok := data["config_version"]; ok {
		switch v := ver.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return DetectVersion(data)
}

func getVersionFromMap(data map[string]interface{}) int {
	if ver, ok := data["config_version"]; ok {
		switch v := ver.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return 0
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case map[string]interface{}:
		// Show first-level keys with their values
		var parts []string
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", k, formatSimpleValue(val[k])))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatSimpleValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%g", val)
	case []interface{}:
		return fmt.Sprintf("[%d items]", len(val))
	case map[string]interface{}:
		return fmt.Sprintf("{%d keys}", len(val))
	default:
		return fmt.Sprintf("%v", v)
	}
}

// detectRenames finds keys that were renamed (same value, different key).
func detectRenames(old, new map[string]interface{}) [][2]string {
	var renames [][2]string

	// Known rename mappings
	knownRenames := map[string]string{
		"apiKey":   "api_key",
		"autoMode": "auto_approve",
	}

	for oldKey, newKey := range knownRenames {
		oldVal, hadOld := old[oldKey]
		newVal, hasNew := new[newKey]
		_, stillHasOld := new[oldKey]
		if hadOld && hasNew && !stillHasOld {
			oldJSON, _ := json.Marshal(oldVal)
			newJSON, _ := json.Marshal(newVal)
			if string(oldJSON) == string(newJSON) {
				renames = append(renames, [2]string{oldKey, newKey})
			}
		}
	}

	return renames
}

// detectMoves finds values that moved from nested to top-level or vice versa.
func detectMoves(old, new map[string]interface{}) [][2]string {
	var moves [][2]string

	// Check provider.model → model
	if provider, ok := old["provider"].(map[string]interface{}); ok {
		if modelVal, ok := provider["model"]; ok {
			if newModel, ok := new["model"]; ok {
				oldJSON, _ := json.Marshal(modelVal)
				newJSON, _ := json.Marshal(newModel)
				if string(oldJSON) == string(newJSON) {
					// Check that provider.model no longer exists in new
					if newProvider, ok := new["provider"].(map[string]interface{}); ok {
						if _, still := newProvider["model"]; !still {
							moves = append(moves, [2]string{"provider.model", "model"})
						}
					} else if _, hasProvider := new["provider"]; !hasProvider {
						moves = append(moves, [2]string{"provider.model", "model"})
					}
				}
			}
		}
	}

	return moves
}
