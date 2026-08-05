package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

// --- sanitizeName tests ---

func TestSanitizeName_ValidName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with-dash", "with-dash"},
		{"with_underscore", "with_underscore"},
		{"with.dot", "with.dot"},
		{"MixedCase", "MixedCase"},
		{"with spaces", "with-spaces"},
		{"with@special#chars", "with-special-chars"},
		{"123numbers", "123numbers"},
		{"", "project"},    // empty becomes "project"
		{"...", "project"}, // only dots becomes "project"
		{"---", "project"}, // only dashes becomes "project"
		{"   ", "project"}, // only spaces becomes "project"
		{".-.", "project"}, // mixed dots and dashes becomes "project"
		{"valid.name-123", "valid.name-123"},
		{"UPPER", "UPPER"},
		{"a", "a"},
	}

	for _, tt := range tests {
		got := sanitizeName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- cleanEnvDir tests ---

func TestCleanEnvDir_Set(t *testing.T) {
	t.Setenv("TEST_VAR", "/some/path")
	got := cleanEnvDir("TEST_VAR")
	if got != "/some/path" {
		t.Errorf("cleanEnvDir() = %q, want %q", got, "/some/path")
	}
}

func TestCleanEnvDir_Unset(t *testing.T) {
	t.Setenv("TEST_VAR", "")
	got := cleanEnvDir("TEST_VAR")
	if got != "" {
		t.Errorf("cleanEnvDir() = %q, want empty string", got)
	}
}

func TestCleanEnvDir_Whitespace(t *testing.T) {
	t.Setenv("TEST_VAR", "  /path/with/spaces  ")
	got := cleanEnvDir("TEST_VAR")
	if got != "/path/with/spaces" {
		t.Errorf("cleanEnvDir() = %q, want %q", got, "/path/with/spaces")
	}
}

// --- ProjectID tests ---

func TestProjectID_EmptyPath(t *testing.T) {
	id := ProjectID("")
	if id == "" {
		t.Error("ProjectID('') should return non-empty string")
	}
	// Empty string becomes "." which uses current directory name as base
	if !strings.Contains(id, "-") {
		t.Errorf("ProjectID('') = %q, want format 'base-hash'", id)
	}
}

func TestProjectID_DotPath(t *testing.T) {
	id := ProjectID(".")
	if id == "" {
		t.Error("ProjectID('.') should return non-empty string")
	}
	// Uses current directory name as base
	if !strings.Contains(id, "-") {
		t.Errorf("ProjectID('.') = %q, want format 'base-hash'", id)
	}
}

func TestProjectID_RootPath(t *testing.T) {
	id := ProjectID("/")
	if id == "" {
		t.Error("ProjectID('/') should return non-empty string")
	}
	if !strings.HasPrefix(id, "project-") {
		t.Errorf("ProjectID('/') = %q, want prefix 'project-'", id)
	}
}

func TestProjectID_DifferentPaths(t *testing.T) {
	id1 := ProjectID("/path/to/project1")
	id2 := ProjectID("/path/to/project2")
	if id1 == id2 {
		t.Error("Different paths should produce different IDs")
	}
}

func TestProjectID_RelativeVsAbsolute(t *testing.T) {
	// Same directory accessed via relative and absolute path should produce same ID
	abs, err := filepath.Abs(".")
	if err != nil {
		t.Skip("Cannot get absolute path")
	}
	id1 := ProjectID(".")
	id2 := ProjectID(abs)
	if id1 != id2 {
		t.Errorf("Relative and absolute paths should produce same ID: %q vs %q", id1, id2)
	}
}

func TestProjectID_Format(t *testing.T) {
	id := ProjectID("/some/project")
	// Should be: sanitized-base + "-" + 12 hex chars
	parts := strings.SplitN(id, "-", 2)
	if len(parts) < 2 {
		t.Fatalf("ProjectID() = %q, want format 'base-hash'", id)
	}
	hash := parts[len(parts)-1]
	if len(hash) != projectIDHashLen {
		t.Errorf("ProjectID() hash length = %d, want %d", len(hash), projectIDHashLen)
	}
	// Hash should be valid hex
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("ProjectID() hash contains invalid hex char: %c", c)
		}
	}
}

// --- Directory path tests ---

func TestSessionsDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(envStateDir, stateDir)

	got := SessionsDir()
	want := filepath.Join(stateDir, "sessions")
	if got != want {
		t.Errorf("SessionsDir() = %q, want %q", got, want)
	}
}

func TestDaemonRunDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(envStateDir, stateDir)

	got := DaemonRunDir()
	want := filepath.Join(stateDir, "run")
	if got != want {
		t.Errorf("DaemonRunDir() = %q, want %q", got, want)
	}
}

func TestWorkspaceSnapshotsDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(envStateDir, stateDir)

	got := WorkspaceSnapshotsDir()
	want := filepath.Join(stateDir, "snapshots")
	if got != want {
		t.Errorf("WorkspaceSnapshotsDir() = %q, want %q", got, want)
	}
}

func TestPersonasDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(envStateDir, stateDir)

	got := PersonasDir()
	want := filepath.Join(stateDir, "agents")
	if got != want {
		t.Errorf("PersonasDir() = %q, want %q", got, want)
	}
}

func TestTasteDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(envStateDir, stateDir)

	got := TasteDir()
	want := filepath.Join(stateDir, "taste")
	if got != want {
		t.Errorf("TasteDir() = %q, want %q", got, want)
	}
}

func TestPlansDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv(envStateDir, stateDir)
	project := filepath.Join(t.TempDir(), "myproject")

	got := PlansDir(project)
	id := ProjectID(project)
	want := filepath.Join(stateDir, "projects", id, "plans")
	if got != want {
		t.Errorf("PlansDir() = %q, want %q", got, want)
	}
}

func TestRepoMapCacheDir(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	t.Setenv(envCacheDir, cacheDir)
	project := filepath.Join(t.TempDir(), "myproject")

	got := RepoMapCacheDir(project)
	id := ProjectID(project)
	want := filepath.Join(cacheDir, "projects", id, "repomap")
	if got != want {
		t.Errorf("RepoMapCacheDir() = %q, want %q", got, want)
	}
}

// --- ConfigDir/StateDir/CacheDir fallback tests ---

func TestConfigDir_Default(t *testing.T) {
	// Clear env var to test default behavior
	t.Setenv(envConfigDir, "")
	t.Setenv(envEyrieConfigDir, "")

	got := ConfigDir()
	if got == "" {
		t.Error("ConfigDir() should return non-empty string")
	}
	// Should end with "hawk"
	if !strings.HasSuffix(got, "hawk") {
		t.Errorf("ConfigDir() = %q, want suffix 'hawk'", got)
	}
}

func TestStateDir_Default(t *testing.T) {
	t.Setenv(envStateDir, "")

	got := StateDir()
	if got == "" {
		t.Error("StateDir() should return non-empty string")
	}
}

func TestCacheDir_Default(t *testing.T) {
	t.Setenv(envCacheDir, "")

	got := CacheDir()
	if got == "" {
		t.Error("CacheDir() should return non-empty string")
	}
}

// --- SettingsPath tests ---

func TestSettingsPath(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "config")
	t.Setenv(envConfigDir, configDir)

	got := SettingsPath()
	want := filepath.Join(configDir, "settings.json")
	if got != want {
		t.Errorf("SettingsPath() = %q, want %q", got, want)
	}
}

// --- ProviderConfigPath tests ---

func TestProviderConfigPath_EyrieOverride(t *testing.T) {
	eyrieDir := filepath.Join(t.TempDir(), "eyrie")
	t.Setenv(envEyrieConfigDir, eyrieDir)

	got := ProviderConfigPath()
	want := filepath.Join(eyrieDir, "provider.json")
	if got != want {
		t.Errorf("ProviderConfigPath() = %q, want %q", got, want)
	}
}

func TestProviderConfigPath_HawkFallback(t *testing.T) {
	hawkDir := filepath.Join(t.TempDir(), "hawk")
	t.Setenv(envConfigDir, hawkDir)
	t.Setenv(envEyrieConfigDir, "")

	got := ProviderConfigPath()
	want := filepath.Join(hawkDir, "provider.json")
	if got != want {
		t.Errorf("ProviderConfigPath() = %q, want %q", got, want)
	}
}

func TestProviderConfigPath_EyrieWhitespaceIgnored(t *testing.T) {
	hawkDir := filepath.Join(t.TempDir(), "hawk")
	t.Setenv(envConfigDir, hawkDir)
	t.Setenv(envEyrieConfigDir, "   ") // whitespace should be treated as empty

	got := ProviderConfigPath()
	want := filepath.Join(hawkDir, "provider.json")
	if got != want {
		t.Errorf("ProviderConfigPath() = %q, want %q", got, want)
	}
}
