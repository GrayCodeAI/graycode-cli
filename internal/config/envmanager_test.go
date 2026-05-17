package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewEnvManager(t *testing.T) {
	em := NewEnvManager()
	if em == nil {
		t.Fatal("NewEnvManager returned nil")
	}
	if em.Vars == nil {
		t.Fatal("Vars map not initialized")
	}
	if em.Profiles == nil {
		t.Fatal("Profiles map not initialized")
	}
	if em.ActiveProfile != "" {
		t.Fatalf("ActiveProfile should be empty, got %q", em.ActiveProfile)
	}
}

func TestSetAndGet(t *testing.T) {
	em := NewEnvManager()

	em.Set("FOO", "bar", false)
	if v := em.Get("FOO"); v != "bar" {
		t.Fatalf("expected 'bar', got %q", v)
	}

	em.Set("SECRET_KEY", "mysecret", true)
	if v := em.Get("SECRET_KEY"); v != "mysecret" {
		t.Fatalf("expected 'mysecret', got %q", v)
	}
	if !em.Vars["SECRET_KEY"].Secret {
		t.Fatal("expected SECRET_KEY to be marked as secret")
	}
}

func TestGetRequired(t *testing.T) {
	em := NewEnvManager()

	// Should fail for missing key
	_, err := em.GetRequired("MISSING_KEY")
	if err == nil {
		t.Fatal("expected error for missing required key")
	}

	// Should succeed for set key
	em.Set("PRESENT", "value", false)
	val, err := em.GetRequired("PRESENT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "value" {
		t.Fatalf("expected 'value', got %q", val)
	}

	// Should fail for empty value
	em.Set("EMPTY", "", false)
	_, err = em.GetRequired("EMPTY")
	if err == nil {
		t.Fatal("expected error for empty required key")
	}
}

func TestGetFallsBackToOSEnv(t *testing.T) {
	em := NewEnvManager()
	key := "TEST_ENV_MANAGER_FALLBACK_KEY"
	os.Setenv(key, "from_os")
	defer os.Unsetenv(key)

	val := em.Get(key)
	if val != "from_os" {
		t.Fatalf("expected 'from_os', got %q", val)
	}
}

func TestParseEnvFile(t *testing.T) {
	// Create a temporary .env file
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	content := `# This is a comment
FOO=bar
BAZ="quoted value"
SINGLE='single quoted'
export EXPORTED=exportval

SPACES =  spaced

MULTI=line1\
line2

EMPTY_LINE_ABOVE=yes
`
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test .env: %v", err)
	}

	result, err := ParseEnvFile(envPath)
	if err != nil {
		t.Fatalf("ParseEnvFile error: %v", err)
	}

	tests := []struct {
		key      string
		expected string
	}{
		{"FOO", "bar"},
		{"BAZ", "quoted value"},
		{"SINGLE", "single quoted"},
		{"EXPORTED", "exportval"},
		{"SPACES", "spaced"},
		{"EMPTY_LINE_ABOVE", "yes"},
	}

	for _, tc := range tests {
		val, ok := result[tc.key]
		if !ok {
			t.Errorf("key %q not found in parsed result", tc.key)
			continue
		}
		if val != tc.expected {
			t.Errorf("key %q: expected %q, got %q", tc.key, tc.expected, val)
		}
	}

	// Check multiline
	multiVal, ok := result["MULTI"]
	if !ok {
		t.Fatal("MULTI key not found")
	}
	if !strings.Contains(multiVal, "line1") || !strings.Contains(multiVal, "line2") {
		t.Errorf("MULTI value unexpected: %q", multiVal)
	}
}

func TestParseEnvFileNotExist(t *testing.T) {
	_, err := ParseEnvFile("/nonexistent/path/.env")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestMaskSecrets(t *testing.T) {
	em := NewEnvManager()
	em.Set("API_KEY", "sk-secret-12345", true)
	em.Set("NORMAL", "visible", false)

	text := "Using API key sk-secret-12345 and normal visible value"
	masked := em.MaskSecrets(text)

	if strings.Contains(masked, "sk-secret-12345") {
		t.Fatalf("secret value should be masked, got: %s", masked)
	}
	if !strings.Contains(masked, "visible") {
		t.Fatalf("non-secret value should NOT be masked, got: %s", masked)
	}
	if !strings.Contains(masked, "***") {
		t.Fatalf("masked text should contain ***, got: %s", masked)
	}
}

func TestListForDisplay(t *testing.T) {
	em := NewEnvManager()
	em.Vars["ANTHROPIC_API_KEY"] = &EnvVar{
		Key:    "ANTHROPIC_API_KEY",
		Value:  "sk-ant-api03-abcdef1234567890abcdef1234567890-3f",
		Source: "env",
		Secret: true,
	}
	em.Vars["HAWK_MODEL"] = &EnvVar{
		Key:    "HAWK_MODEL",
		Value:  "claude-sonnet-4-6",
		Source: ".env.local",
		Secret: false,
	}

	output := em.ListForDisplay()

	if !strings.Contains(output, "Environment Variables:") {
		t.Fatal("output should contain header")
	}
	if !strings.Contains(output, "ANTHROPIC_API_KEY") {
		t.Fatal("output should contain ANTHROPIC_API_KEY")
	}
	if !strings.Contains(output, "secret") {
		t.Fatal("output should indicate secret")
	}
	if !strings.Contains(output, "from: env") {
		t.Fatal("output should show source")
	}
	if !strings.Contains(output, "HAWK_MODEL") {
		t.Fatal("output should contain HAWK_MODEL")
	}
	if !strings.Contains(output, "claude-sonnet-4-6") {
		t.Fatal("output should show non-secret value in full")
	}
	// Secret value should be masked
	if strings.Contains(output, "sk-ant-api03-abcdef1234567890abcdef1234567890-3f") {
		t.Fatal("secret value should be masked in display")
	}
}

func TestListForDisplayEmpty(t *testing.T) {
	em := NewEnvManager()
	output := em.ListForDisplay()
	if !strings.Contains(output, "(none)") {
		t.Fatalf("expected '(none)' for empty manager, got: %s", output)
	}
}

func TestValidate(t *testing.T) {
	em := NewEnvManager()
	em.Vars["REQUIRED_KEY"] = &EnvVar{
		Key:      "REQUIRED_KEY",
		Value:    "",
		Source:   "default",
		Required: true,
	}
	em.Vars["OPTIONAL_KEY"] = &EnvVar{
		Key:    "OPTIONAL_KEY",
		Value:  "set",
		Source: "env",
	}

	warnings := em.Validate()
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "REQUIRED_KEY") && strings.Contains(w, "ERROR") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected error about REQUIRED_KEY, got: %v", warnings)
	}
}

func TestSaveAndLoadProfile(t *testing.T) {
	em := NewEnvManager()
	em.Set("DB_HOST", "localhost", false)
	em.Set("DB_PORT", "5432", false)
	em.Set("API_KEY", "secret123", true)

	// Save profile
	err := em.SaveProfile("development", []string{"DB_HOST", "DB_PORT", "API_KEY"})
	if err != nil {
		t.Fatalf("SaveProfile error: %v", err)
	}

	// Verify profile saved
	if _, ok := em.Profiles["development"]; !ok {
		t.Fatal("profile 'development' not saved")
	}

	// Load profile
	err = em.LoadProfile("development")
	if err != nil {
		t.Fatalf("LoadProfile error: %v", err)
	}
	if em.ActiveProfile != "development" {
		t.Fatalf("expected active profile 'development', got %q", em.ActiveProfile)
	}
}

func TestLoadProfileNotFound(t *testing.T) {
	em := NewEnvManager()
	err := em.LoadProfile("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
}

func TestSaveProfileEmptyName(t *testing.T) {
	em := NewEnvManager()
	err := em.SaveProfile("", []string{"FOO"})
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
}

func TestDiff(t *testing.T) {
	em1 := NewEnvManager()
	em1.Set("SHARED", "value1", false)
	em1.Set("ONLY_IN_A", "a", false)
	em1.Set("DIFFERENT", "old", false)

	em2 := NewEnvManager()
	em2.Set("SHARED", "value1", false)
	em2.Set("ONLY_IN_B", "b", false)
	em2.Set("DIFFERENT", "new", false)

	diffs := em1.Diff(em2)

	if len(diffs) == 0 {
		t.Fatal("expected differences")
	}

	foundOnlyA := false
	foundOnlyB := false
	foundDifferent := false

	for _, d := range diffs {
		if strings.Contains(d, "ONLY_IN_A") && strings.HasPrefix(d, "+") {
			foundOnlyA = true
		}
		if strings.Contains(d, "ONLY_IN_B") && strings.HasPrefix(d, "-") {
			foundOnlyB = true
		}
		if strings.Contains(d, "DIFFERENT") && strings.HasPrefix(d, "~") {
			foundDifferent = true
		}
	}

	if !foundOnlyA {
		t.Error("expected diff to show ONLY_IN_A as added")
	}
	if !foundOnlyB {
		t.Error("expected diff to show ONLY_IN_B as removed")
	}
	if !foundDifferent {
		t.Error("expected diff to show DIFFERENT as changed")
	}
}

func TestDiffNoDifferences(t *testing.T) {
	em1 := NewEnvManager()
	em1.Set("KEY", "value", false)

	em2 := NewEnvManager()
	em2.Set("KEY", "value", false)

	diffs := em1.Diff(em2)
	if len(diffs) != 0 {
		t.Fatalf("expected no diffs, got: %v", diffs)
	}
}

func TestExportEnvFormat(t *testing.T) {
	em := NewEnvManager()
	em.Set("FOO", "bar", false)
	em.Set("BAZ", "qux", false)

	output := em.Export("env")
	if !strings.Contains(output, "FOO=bar") {
		t.Fatalf("env format should contain 'FOO=bar', got: %s", output)
	}
	if !strings.Contains(output, "BAZ=qux") {
		t.Fatalf("env format should contain 'BAZ=qux', got: %s", output)
	}
}

func TestExportJSONFormat(t *testing.T) {
	em := NewEnvManager()
	em.Set("KEY1", "value1", false)
	em.Set("KEY2", "value2", false)

	output := em.Export("json")
	if !strings.Contains(output, `"KEY1"`) {
		t.Fatalf("JSON format should contain key, got: %s", output)
	}
	if !strings.Contains(output, `"value1"`) {
		t.Fatalf("JSON format should contain value, got: %s", output)
	}

	// Verify it's valid JSON
	var parsed map[string]string
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("JSON output should be valid JSON: %v", err)
	}
	if parsed["KEY1"] != "value1" {
		t.Fatalf("expected KEY1=value1, got %q", parsed["KEY1"])
	}
}

func TestExportShellFormat(t *testing.T) {
	em := NewEnvManager()
	em.Set("SIMPLE", "value", false)
	em.Set("WITH_SPACES", "hello world", false)

	output := em.Export("shell")
	if !strings.Contains(output, "export SIMPLE=value") {
		t.Fatalf("shell format should contain 'export SIMPLE=value', got: %s", output)
	}
	if !strings.Contains(output, "export WITH_SPACES=") {
		t.Fatalf("shell format should contain 'export WITH_SPACES=...', got: %s", output)
	}
	// Value with spaces should be quoted
	if !strings.Contains(output, `"hello world"`) {
		t.Fatalf("shell format should quote value with spaces, got: %s", output)
	}
}

func TestLoad(t *testing.T) {
	// Create temp directory with .env file
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "LOADED_KEY=loaded_value\nSECOND=two\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	em := NewEnvManager()
	err := em.Load(envPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if v := em.Get("LOADED_KEY"); v != "loaded_value" {
		t.Fatalf("expected 'loaded_value', got %q", v)
	}
}

func TestLoadOSEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "OVERRIDE_TEST=from_file\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// Set OS env to override
	os.Setenv("OVERRIDE_TEST", "from_os")
	defer os.Unsetenv("OVERRIDE_TEST")

	em := NewEnvManager()
	err := em.Load(envPath)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if v := em.Get("OVERRIDE_TEST"); v != "from_os" {
		t.Fatalf("OS env should override file, expected 'from_os', got %q", v)
	}
	if em.Vars["OVERRIDE_TEST"].Source != "env" {
		t.Fatalf("source should be 'env', got %q", em.Vars["OVERRIDE_TEST"].Source)
	}
}

func TestLoadMultipleSources(t *testing.T) {
	dir := t.TempDir()

	// Lower priority source
	low := filepath.Join(dir, "global.env")
	os.WriteFile(low, []byte("KEY=global\nGLOBAL_ONLY=yes\n"), 0o644)

	// Higher priority source (loaded later)
	high := filepath.Join(dir, ".env")
	os.WriteFile(high, []byte("KEY=project\nPROJECT_ONLY=yes\n"), 0o644)

	em := NewEnvManager()
	// Sources in order: lowest priority first
	err := em.Load(low, high)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// KEY should have the higher-priority value
	if v := em.Get("KEY"); v != "project" {
		t.Fatalf("expected 'project' (higher priority), got %q", v)
	}
	if v := em.Get("GLOBAL_ONLY"); v != "yes" {
		t.Fatalf("expected 'yes', got %q", v)
	}
	if v := em.Get("PROJECT_ONLY"); v != "yes" {
		t.Fatalf("expected 'yes', got %q", v)
	}
}

func TestMaskSecretsEmptyValue(t *testing.T) {
	em := NewEnvManager()
	em.Vars["EMPTY_SECRET"] = &EnvVar{
		Key:    "EMPTY_SECRET",
		Value:  "",
		Secret: true,
		Source: "env",
	}

	// Should not panic or corrupt text with empty secret value
	text := "some normal text"
	masked := em.MaskSecrets(text)
	if masked != text {
		t.Fatalf("masking empty secret should not change text, got: %s", masked)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"", `""`},
		{"hello world", `"hello world"`},
		{`has"quote`, `"has\"quote"`},
		{"has$dollar", `"has\$dollar"`},
	}

	for _, tc := range tests {
		result := shellQuote(tc.input)
		if result != tc.expected {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.input, result, tc.expected)
		}
	}
}

func TestSourceNameFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{".env", ".env"},
		{"/project/.env", ".env"},
		{".env.local", ".env.local"},
		{"/home/user/.hawk/env", "~/.hawk/env"},
		{"/some/random/file.txt", "file"},
	}

	for _, tc := range tests {
		result := sourceNameFromPath(tc.path)
		if result != tc.expected {
			t.Errorf("sourceNameFromPath(%q) = %q, want %q", tc.path, result, tc.expected)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	em := NewEnvManager()
	done := make(chan bool, 10)

	// Concurrent writes
	for i := 0; i < 5; i++ {
		go func(n int) {
			em.Set(strings.Repeat("K", n+1), "value", false)
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 5; i++ {
		go func(n int) {
			_ = em.Get(strings.Repeat("K", n+1))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
