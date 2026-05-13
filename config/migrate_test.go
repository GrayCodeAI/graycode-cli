package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationV1ToV2(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"apiKey":         "sk-test-123",
		"config_version": 1,
	}

	result, err := r.Migrations[0].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := result["apiKey"]; ok {
		t.Error("apiKey should have been removed")
	}
	if val, ok := result["api_key"]; !ok || val != "sk-test-123" {
		t.Errorf("api_key should be 'sk-test-123', got %v", val)
	}
}

func TestMigrationV2ToV3(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"provider": map[string]interface{}{
			"model": "claude-3-opus",
			"name":  "anthropic",
		},
		"config_version": 2,
	}

	result, err := r.Migrations[1].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val, ok := result["model"]; !ok || val != "claude-3-opus" {
		t.Errorf("model should be 'claude-3-opus', got %v", val)
	}

	// Provider should still exist but without model
	provider, ok := result["provider"].(map[string]interface{})
	if !ok {
		t.Fatal("provider should still exist")
	}
	if _, ok := provider["model"]; ok {
		t.Error("provider.model should have been removed")
	}
	if provider["name"] != "anthropic" {
		t.Error("provider.name should still be 'anthropic'")
	}
}

func TestMigrationV2ToV3_EmptyProvider(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"provider": map[string]interface{}{
			"model": "gpt-4",
		},
		"config_version": 2,
	}

	result, err := r.Migrations[1].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if val, ok := result["model"]; !ok || val != "gpt-4" {
		t.Errorf("model should be 'gpt-4', got %v", val)
	}

	// Provider should be removed entirely since it's empty
	if _, ok := result["provider"]; ok {
		t.Error("empty provider object should have been removed")
	}
}

func TestMigrationV3ToV4(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"model":          "claude-3-opus",
		"config_version": 3,
	}

	result, err := r.Migrations[2].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perms, ok := result["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("permissions section should be added")
	}
	if perms["allow_read"] != true {
		t.Error("permissions.allow_read should default to true")
	}
	if perms["allow_execute"] != false {
		t.Error("permissions.allow_execute should default to false")
	}
}

func TestMigrationV3ToV4_ExistingPermissions(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow_read": false,
		},
		"config_version": 3,
	}

	result, err := r.Migrations[2].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	perms, ok := result["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("permissions section should exist")
	}
	// Should preserve existing value
	if perms["allow_read"] != false {
		t.Error("existing permissions.allow_read should be preserved as false")
	}
}

func TestMigrationV4ToV5(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"autoMode":       true,
		"config_version": 4,
	}

	result, err := r.Migrations[3].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := result["autoMode"]; ok {
		t.Error("autoMode should have been removed")
	}
	if val, ok := result["auto_approve"]; !ok || val != true {
		t.Error("auto_approve should be true")
	}

	guardian, ok := result["guardian"].(map[string]interface{})
	if !ok {
		t.Fatal("guardian section should be added")
	}
	if guardian["enabled"] != true {
		t.Error("guardian.enabled should default to true")
	}
	if guardian["max_risk"] != "medium" {
		t.Error("guardian.max_risk should default to 'medium'")
	}
}

func TestMigrationV5ToV6(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"config_version": 5,
	}

	result, err := r.Migrations[4].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok, ok := result["tok"].(map[string]interface{})
	if !ok {
		t.Fatal("tok section should be added")
	}
	if tok["compression_mode"] != "full" {
		t.Error("tok.compression_mode should default to 'full'")
	}
	if tok["max_tokens"] != 100000 {
		t.Error("tok.max_tokens should default to 100000")
	}
	if tok["preserve_code"] != true {
		t.Error("tok.preserve_code should default to true")
	}
}

func TestMigrationV6ToV7(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"config_version": 6,
	}

	result, err := r.Migrations[5].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cw, ok := result["cache_warming"].(map[string]interface{})
	if !ok {
		t.Fatal("cache_warming section should be added")
	}
	if cw["enabled"] != false {
		t.Error("cache_warming.enabled should default to false")
	}
	if cw["strategy"] != "recent_files" {
		t.Error("cache_warming.strategy should default to 'recent_files'")
	}

	cl, ok := result["cost_limits"].(map[string]interface{})
	if !ok {
		t.Fatal("cost_limits section should be added")
	}
	if cl["max_per_session"] != 5.0 {
		t.Error("cost_limits.max_per_session should default to 5.0")
	}
	if cl["hard_stop"] != true {
		t.Error("cost_limits.hard_stop should default to true")
	}
}

func TestMigrationV7ToV8_BoolSandbox(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"sandbox":        true,
		"config_version": 7,
	}

	result, err := r.Migrations[6].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sandbox, ok := result["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("sandbox should be converted to an object")
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled should be true")
	}
	if sandbox["type"] != "landlock" {
		t.Error("sandbox.type should default to 'landlock'")
	}
	if sandbox["network"] != false {
		t.Error("sandbox.network should default to false")
	}
}

func TestMigrationV7ToV8_StringSandbox(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"sandbox":        "docker",
		"config_version": 7,
	}

	result, err := r.Migrations[6].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sandbox, ok := result["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("sandbox should be converted to an object")
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled should be true")
	}
	if sandbox["type"] != "docker" {
		t.Errorf("sandbox.type should be 'docker', got %v", sandbox["type"])
	}
}

func TestMigrationV7ToV8_NoSandbox(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"config_version": 7,
	}

	result, err := r.Migrations[6].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sandbox, ok := result["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("sandbox should be created as an object")
	}
	if sandbox["enabled"] != false {
		t.Error("sandbox.enabled should be false when not previously set")
	}
}

func TestMigrationV7ToV8_AlreadyObject(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"sandbox": map[string]interface{}{
			"enabled": true,
		},
		"config_version": 7,
	}

	result, err := r.Migrations[6].Migrate(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sandbox, ok := result["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("sandbox should remain an object")
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled should be preserved as true")
	}
	if sandbox["type"] != "landlock" {
		t.Error("sandbox.type should be added as 'landlock'")
	}
	if sandbox["network"] != false {
		t.Error("sandbox.network should be added as false")
	}
}

func TestFullMigrationChainV1ToV8(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"apiKey": "sk-test",
		"provider": map[string]interface{}{
			"model": "claude-3-opus",
			"name":  "anthropic",
		},
		"autoMode":       true,
		"sandbox":        true,
		"config_version": 1,
	}

	result, err := r.Run(data)
	if err != nil {
		t.Fatalf("full migration failed: %v", err)
	}

	// Check v1→v2
	if _, ok := result["apiKey"]; ok {
		t.Error("apiKey should be renamed to api_key")
	}
	if result["api_key"] != "sk-test" {
		t.Error("api_key should have value 'sk-test'")
	}

	// Check v2→v3
	if result["model"] != "claude-3-opus" {
		t.Error("model should be moved to top level")
	}

	// Check v3→v4
	if _, ok := result["permissions"].(map[string]interface{}); !ok {
		t.Error("permissions section should exist")
	}

	// Check v4→v5
	if _, ok := result["autoMode"]; ok {
		t.Error("autoMode should be renamed to auto_approve")
	}
	if result["auto_approve"] != true {
		t.Error("auto_approve should be true")
	}
	if _, ok := result["guardian"].(map[string]interface{}); !ok {
		t.Error("guardian section should exist")
	}

	// Check v5→v6
	if _, ok := result["tok"].(map[string]interface{}); !ok {
		t.Error("tok section should exist")
	}

	// Check v6→v7
	if _, ok := result["cache_warming"].(map[string]interface{}); !ok {
		t.Error("cache_warming section should exist")
	}
	if _, ok := result["cost_limits"].(map[string]interface{}); !ok {
		t.Error("cost_limits section should exist")
	}

	// Check v7→v8
	sandbox, ok := result["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("sandbox should be an object")
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled should be true")
	}

	// Check final version
	ver, ok := result["config_version"]
	if !ok {
		t.Fatal("config_version should be set")
	}
	if v, ok := ver.(int); ok {
		if v != 8 {
			t.Errorf("config_version should be 8, got %d", v)
		}
	} else {
		t.Errorf("config_version should be int, got %T", ver)
	}
}

func TestNeedsMigration(t *testing.T) {
	r := NewMigrationRegistry()

	tests := []struct {
		name   string
		data   map[string]interface{}
		expect bool
	}{
		{
			name:   "version 1 needs migration",
			data:   map[string]interface{}{"config_version": 1},
			expect: true,
		},
		{
			name:   "version 7 needs migration",
			data:   map[string]interface{}{"config_version": 7},
			expect: true,
		},
		{
			name:   "version 8 does not need migration",
			data:   map[string]interface{}{"config_version": 8},
			expect: false,
		},
		{
			name:   "no version field (has apiKey) needs migration",
			data:   map[string]interface{}{"apiKey": "test"},
			expect: true,
		},
		{
			name:   "higher version does not need migration",
			data:   map[string]interface{}{"config_version": 99},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.NeedsMigration(tt.data)
			if got != tt.expect {
				t.Errorf("NeedsMigration() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestBackupCreation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	original := `{"config_version": 1, "apiKey": "test"}`
	if err := os.WriteFile(configPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewMigrationRegistry()
	backupPath, err := r.Backup(configPath)
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatal("backup file should exist")
	}

	// Verify backup content matches original
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(backupData) != original {
		t.Error("backup content should match original")
	}

	// Verify backup path format
	if !strings.HasPrefix(backupPath, configPath+".bak.") {
		t.Errorf("backup path should start with %s.bak., got %s", configPath, backupPath)
	}
}

func TestBackupNonexistentFile(t *testing.T) {
	r := NewMigrationRegistry()
	_, err := r.Backup("/tmp/nonexistent_config_12345.json")
	if err == nil {
		t.Error("backup of nonexistent file should fail")
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	backupPath := filepath.Join(dir, "config.json.bak.20240315_103045")

	original := `{"config_version": 1, "apiKey": "original"}`
	migrated := `{"config_version": 8, "api_key": "original"}`

	if err := os.WriteFile(backupPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(migrated), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RollbackMigration(configPath, backupPath); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Verify config was restored
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Error("config should be restored to original content")
	}
}

func TestRollbackMissingBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RollbackMigration(configPath, "/tmp/nonexistent_backup.json")
	if err == nil {
		t.Error("rollback with missing backup should fail")
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	data := map[string]interface{}{
		"config_version": 8,
		"permissions":    map[string]interface{}{"allow_read": true},
		"guardian":       map[string]interface{}{"enabled": true},
		"tok":            map[string]interface{}{"compression_mode": "full"},
		"cache_warming":  map[string]interface{}{"enabled": false},
		"cost_limits":    map[string]interface{}{"max_per_session": 5.0},
		"sandbox":        map[string]interface{}{"enabled": true, "type": "landlock"},
	}

	warnings := ValidateConfig(data)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
}

func TestValidateConfig_MissingSections(t *testing.T) {
	data := map[string]interface{}{
		"config_version": 8,
	}

	warnings := ValidateConfig(data)
	if len(warnings) == 0 {
		t.Error("expected warnings for missing sections")
	}

	// Should warn about all required sections
	joined := strings.Join(warnings, " ")
	for _, section := range []string{"permissions", "guardian", "tok", "cache_warming", "cost_limits", "sandbox"} {
		if !strings.Contains(joined, section) {
			t.Errorf("should warn about missing %s", section)
		}
	}
}

func TestValidateConfig_DeprecatedFields(t *testing.T) {
	data := map[string]interface{}{
		"config_version": 8,
		"apiKey":         "should-not-be-here",
		"autoMode":       true,
		"permissions":    map[string]interface{}{},
		"guardian":       map[string]interface{}{},
		"tok":            map[string]interface{}{},
		"cache_warming":  map[string]interface{}{},
		"cost_limits":    map[string]interface{}{},
		"sandbox":        map[string]interface{}{},
	}

	warnings := ValidateConfig(data)
	joined := strings.Join(warnings, " ")
	if !strings.Contains(joined, "apiKey") {
		t.Error("should warn about deprecated apiKey")
	}
	if !strings.Contains(joined, "autoMode") {
		t.Error("should warn about deprecated autoMode")
	}
}

func TestValidateConfig_TypeErrors(t *testing.T) {
	data := map[string]interface{}{
		"config_version": 8,
		"permissions":    "should-be-object",
		"guardian":       map[string]interface{}{},
		"tok":            map[string]interface{}{},
		"cache_warming":  map[string]interface{}{},
		"cost_limits":    map[string]interface{}{},
		"sandbox":        "should-be-object",
	}

	warnings := ValidateConfig(data)
	joined := strings.Join(warnings, " ")
	if !strings.Contains(joined, "permissions should be an object") {
		t.Error("should warn about permissions type")
	}
	if !strings.Contains(joined, "sandbox should be an object") {
		t.Error("should warn about sandbox type")
	}
}

func TestDiffConfigs(t *testing.T) {
	old := map[string]interface{}{
		"apiKey":         "test",
		"autoMode":       true,
		"config_version": 1,
		"provider": map[string]interface{}{
			"model": "claude-3-opus",
		},
	}
	new := map[string]interface{}{
		"api_key":        "test",
		"auto_approve":   true,
		"model":          "claude-3-opus",
		"permissions":    map[string]interface{}{},
		"guardian":       map[string]interface{}{},
		"tok":            map[string]interface{}{},
		"cache_warming":  map[string]interface{}{},
		"cost_limits":    map[string]interface{}{},
		"sandbox":        map[string]interface{}{},
		"config_version": 8,
	}

	diff := DiffConfigs(old, new)

	if !strings.Contains(diff, "v1 → v8") {
		t.Error("diff should show version range")
	}
	if !strings.Contains(diff, "Added") {
		t.Error("diff should show added sections")
	}
	if !strings.Contains(diff, "Renamed: apiKey → api_key") {
		t.Error("diff should show renamed fields")
	}
	if !strings.Contains(diff, "Moved: provider.model → model") {
		t.Error("diff should show moved fields")
	}
	if !strings.Contains(diff, "Removed: autoMode") {
		t.Error("diff should show removed fields")
	}
}

func TestDetectVersion(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]interface{}
		version int
	}{
		{
			name:    "explicit version",
			data:    map[string]interface{}{"config_version": float64(5)},
			version: 5,
		},
		{
			name:    "has apiKey (camelCase) → v1",
			data:    map[string]interface{}{"apiKey": "test"},
			version: 1,
		},
		{
			name:    "has api_key (snake_case) → v2",
			data:    map[string]interface{}{"api_key": "test"},
			version: 2,
		},
		{
			name: "has provider.model nested → v2",
			data: map[string]interface{}{
				"provider": map[string]interface{}{"model": "gpt-4"},
			},
			version: 2,
		},
		{
			name:    "has top-level model only → v3",
			data:    map[string]interface{}{"model": "claude-3-opus"},
			version: 3,
		},
		{
			name:    "has permissions → v4",
			data:    map[string]interface{}{"permissions": map[string]interface{}{}},
			version: 4,
		},
		{
			name:    "has guardian → v5",
			data:    map[string]interface{}{"guardian": map[string]interface{}{}},
			version: 5,
		},
		{
			name:    "has auto_approve → v5",
			data:    map[string]interface{}{"auto_approve": true},
			version: 5,
		},
		{
			name:    "has tok → v6",
			data:    map[string]interface{}{"tok": map[string]interface{}{}},
			version: 6,
		},
		{
			name:    "has cache_warming → v7",
			data:    map[string]interface{}{"cache_warming": map[string]interface{}{}},
			version: 7,
		},
		{
			name:    "has cost_limits → v7",
			data:    map[string]interface{}{"cost_limits": map[string]interface{}{}},
			version: 7,
		},
		{
			name:    "has sandbox as object → v8",
			data:    map[string]interface{}{"sandbox": map[string]interface{}{"enabled": true}},
			version: 8,
		},
		{
			name:    "empty config → v1",
			data:    map[string]interface{}{},
			version: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectVersion(tt.data)
			if got != tt.version {
				t.Errorf("DetectVersion() = %d, want %d", got, tt.version)
			}
		})
	}
}

func TestAlreadyCurrentConfigIsNoop(t *testing.T) {
	r := NewMigrationRegistry()
	data := map[string]interface{}{
		"config_version": 8,
		"model":          "claude-3-opus",
		"permissions":    map[string]interface{}{"allow_read": true},
	}

	// Make a copy to compare
	original, _ := json.Marshal(data)

	result, err := r.Run(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resultJSON, _ := json.Marshal(result)
	if string(original) != string(resultJSON) {
		t.Error("already-current config should not be modified")
	}
}

func TestInvalidConfigData(t *testing.T) {
	r := NewMigrationRegistry()

	// Config with no recognizable fields - defaults to v1, should still migrate
	data := map[string]interface{}{
		"random_field":   "value",
		"config_version": 1,
	}

	result, err := r.Run(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have migrated to v8 with all default sections added
	if result["config_version"] != 8 {
		t.Errorf("should migrate to v8, got %v", result["config_version"])
	}
}

func TestMigrateFileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	originalConfig := map[string]interface{}{
		"config_version": 1,
		"apiKey":         "sk-secret",
		"provider": map[string]interface{}{
			"model": "claude-3-opus",
			"name":  "anthropic",
		},
		"autoMode": true,
		"sandbox":  true,
	}

	data, err := json.MarshalIndent(originalConfig, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewMigrationRegistry()
	if err := r.MigrateFile(configPath); err != nil {
		t.Fatalf("MigrateFile failed: %v", err)
	}

	// Read back the migrated config
	migratedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(migratedData, &result); err != nil {
		t.Fatalf("failed to parse migrated config: %v", err)
	}

	// Verify version
	if result["config_version"] != float64(8) {
		t.Errorf("config_version should be 8, got %v", result["config_version"])
	}

	// Verify migrations applied
	if _, ok := result["apiKey"]; ok {
		t.Error("apiKey should have been renamed")
	}
	if result["api_key"] != "sk-secret" {
		t.Error("api_key should be 'sk-secret'")
	}
	if result["model"] != "claude-3-opus" {
		t.Error("model should be at top level")
	}

	// Verify sandbox is now an object
	sandbox, ok := result["sandbox"].(map[string]interface{})
	if !ok {
		t.Fatal("sandbox should be an object")
	}
	if sandbox["enabled"] != true {
		t.Error("sandbox.enabled should be true")
	}

	// Verify backup was created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	foundBackup := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "config.json.bak.") {
			foundBackup = true
			break
		}
	}
	if !foundBackup {
		t.Error("backup file should have been created")
	}
}

func TestMigrateFileAlreadyCurrent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	currentConfig := map[string]interface{}{
		"config_version": 8,
		"model":          "claude-3-opus",
	}

	data, err := json.MarshalIndent(currentConfig, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewMigrationRegistry()
	if err := r.MigrateFile(configPath); err != nil {
		t.Fatalf("MigrateFile should succeed for current config: %v", err)
	}

	// Verify no backup was created (no migration needed)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "config.json.bak.") {
			t.Error("no backup should be created when no migration is needed")
		}
	}
}

func TestMigrateFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(configPath, []byte("not valid json {{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewMigrationRegistry()
	err := r.MigrateFile(configPath)
	if err == nil {
		t.Error("MigrateFile should fail for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parsing, got: %v", err)
	}
}

func TestMigrateFileNonexistent(t *testing.T) {
	r := NewMigrationRegistry()
	err := r.MigrateFile("/tmp/nonexistent_config_file_xyz.json")
	if err == nil {
		t.Error("MigrateFile should fail for nonexistent file")
	}
}

func TestRunPartialMigration(t *testing.T) {
	r := NewMigrationRegistry()

	// Start at v4, should only apply v4→v5, v5→v6, v6→v7, v7→v8
	data := map[string]interface{}{
		"config_version": 4,
		"api_key":        "test",
		"model":          "gpt-4",
		"permissions":    map[string]interface{}{"allow_read": true},
		"autoMode":       false,
	}

	result, err := r.Run(data)
	if err != nil {
		t.Fatalf("partial migration failed: %v", err)
	}

	if result["config_version"] != 8 {
		t.Errorf("should end at v8, got %v", result["config_version"])
	}

	// v4→v5 should have renamed autoMode
	if _, ok := result["autoMode"]; ok {
		t.Error("autoMode should be renamed")
	}
	if result["auto_approve"] != false {
		t.Error("auto_approve should be false")
	}

	// Original fields from pre-v4 should be untouched
	if result["api_key"] != "test" {
		t.Error("api_key should be preserved")
	}
	if result["model"] != "gpt-4" {
		t.Error("model should be preserved")
	}
}

func TestDiffConfigsNoChanges(t *testing.T) {
	data := map[string]interface{}{
		"config_version": 8,
		"model":          "test",
	}

	diff := DiffConfigs(data, data)
	if !strings.Contains(diff, "no changes") {
		t.Error("diff of identical configs should show no changes")
	}
}

func TestValidateConfigVersionType(t *testing.T) {
	data := map[string]interface{}{
		"config_version": "eight", // wrong type
		"permissions":    map[string]interface{}{},
		"guardian":       map[string]interface{}{},
		"tok":            map[string]interface{}{},
		"cache_warming":  map[string]interface{}{},
		"cost_limits":    map[string]interface{}{},
		"sandbox":        map[string]interface{}{},
	}

	warnings := ValidateConfig(data)
	joined := strings.Join(warnings, " ")
	if !strings.Contains(joined, "config_version should be an integer") {
		t.Error("should warn about config_version type")
	}
}

func TestCostLimitsTypeValidation(t *testing.T) {
	data := map[string]interface{}{
		"config_version": 8,
		"permissions":    map[string]interface{}{},
		"guardian":       map[string]interface{}{},
		"tok":            map[string]interface{}{},
		"cache_warming":  map[string]interface{}{},
		"cost_limits": map[string]interface{}{
			"max_per_session": "not-a-number",
		},
		"sandbox": map[string]interface{}{},
	}

	warnings := ValidateConfig(data)
	joined := strings.Join(warnings, " ")
	if !strings.Contains(joined, "max_per_session should be a number") {
		t.Error("should warn about cost_limits.max_per_session type")
	}
}
