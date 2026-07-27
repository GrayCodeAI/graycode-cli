package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveMode_Default tests the default mode resolution.
func TestResolveMode_Default(t *testing.T) {
	cfg := TOMLConfig{
		Profiles: map[string]ProfileConfig{
			ProfileWorkspace: {Mode: "strict"},
		},
	}
	mode := resolveMode(cfg, "")
	if mode != ModeStrict {
		t.Errorf("expected ModeStrict, got %v", mode)
	}
}

// TestResolveMode_UnknownProfile tests fallback to ParseMode for unknown profiles.
func TestResolveMode_UnknownProfile(t *testing.T) {
	cfg := TOMLConfig{}
	mode := resolveMode(cfg, "unknown")
	if mode != ModeStrict {
		t.Errorf("expected ModeStrict for unknown profile, got %v", mode)
	}
}

// TestResolveMode_Aliases tests builtin profile name aliases.
func TestResolveMode_Aliases(t *testing.T) {
	cfg := TOMLConfig{}
	tests := []struct {
		name     string
		expected Mode
	}{
		{"readonly", ModeStrict},
		{"read_only", ModeStrict},
	}

	for _, tc := range tests {
		mode := resolveMode(cfg, tc.name)
		if mode != tc.expected {
			t.Errorf("resolveMode(%q) = %v, want %v", tc.name, mode, tc.expected)
		}
	}
}

// TestResolveMode_Extends tests profile extension.
func TestResolveMode_Extends(t *testing.T) {
	cfg := TOMLConfig{
		Profiles: map[string]ProfileConfig{
			"base":   {Mode: "strict"},
			"child":  {Extends: "base"},
		},
	}
	mode := resolveMode(cfg, "child")
	if mode != ModeStrict {
		t.Errorf("expected ModeStrict for extended profile, got %v", mode)
	}
}

// TestResolveMode_EmptyMode tests default mode for empty mode string.
func TestResolveMode_EmptyMode(t *testing.T) {
	cfg := TOMLConfig{
		Profiles: map[string]ProfileConfig{
			"custom": {Mode: ""},
		},
	}
	mode := resolveMode(cfg, "custom")
	if mode != ModeWorkspace {
		t.Errorf("expected ModeWorkspace for empty mode, got %v", mode)
	}
}

// TestWeaker tests the weaker isolation comparison.
func TestWeaker(t *testing.T) {
	tests := []struct {
		a, b     Mode
		expected bool
	}{
		{ModeOff, ModeWorkspace, true},
		{ModeWorkspace, ModeStrict, true},
		{ModeOff, ModeStrict, true},
		{ModeWorkspace, ModeOff, false},
		{ModeStrict, ModeWorkspace, false},
		{ModeWorkspace, ModeWorkspace, false},
		{ModeOff, ModeOff, false},
		{ModeStrict, ModeStrict, false},
	}

	for _, tc := range tests {
		result := weaker(tc.a, tc.b)
		if result != tc.expected {
			t.Errorf("weaker(%v, %v) = %v, want %v", tc.a, tc.b, result, tc.expected)
		}
	}
}

// TestEffectiveFrom_StrictMode tests strict mode config.
func TestEffectiveFrom_StrictMode(t *testing.T) {
	cfg := TOMLConfig{
		Profile: "strict",
	}
	eff, err := EffectiveFrom(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff.Mode != ModeStrict {
		t.Errorf("expected ModeStrict, got %v", eff.Mode)
	}
}

// TestEffectiveFrom_AllowNetworkOverride tests network override.
func TestEffectiveFrom_AllowNetworkOverride(t *testing.T) {
	cfg := TOMLConfig{
		Profile: "strict",
		Profiles: map[string]ProfileConfig{
			"strict": {
				Mode:         "strict",
				AllowNetwork: boolPtr(false),
			},
		},
	}
	eff, err := EffectiveFrom(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff.AllowNetwork {
		t.Error("expected AllowNetwork to be false")
	}
}

// TestEffectiveFrom_Extends tests profile extension in EffectiveFrom.
func TestEffectiveFrom_Extends(t *testing.T) {
	cfg := TOMLConfig{
		Profile: "child",
		Profiles: map[string]ProfileConfig{
			"base": {
				Mode:         "strict",
				AllowNetwork: boolPtr(false),
				DenyGlobs:    []string{"*.log"},
			},
			"child": {
				Extends: "base",
				Mode:    "", // empty mode inherits from base
			},
		},
	}
	eff, err := EffectiveFrom(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff.Mode != ModeStrict {
		t.Errorf("expected ModeStrict, got %v", eff.Mode)
	}
	if eff.AllowNetwork {
		t.Error("expected AllowNetwork to be false (inherited from base)")
	}
}

// TestEffectiveFrom_DenyGlobs tests deny glob inheritance.
func TestEffectiveFrom_DenyGlobs(t *testing.T) {
	cfg := TOMLConfig{
		DenyGlobs: []string{"*.tmp"},
		Profiles: map[string]ProfileConfig{
			"custom": {
				Mode:      "workspace",
				DenyGlobs: []string{"*.log"},
			},
		},
		Profile: "custom",
	}
	eff, err := EffectiveFrom(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(eff.DenyGlobs) != 2 {
		t.Errorf("expected 2 deny globs, got %d: %v", len(eff.DenyGlobs), eff.DenyGlobs)
	}
}

// TestLoadTOML_ParseError verifies parsing errors are returned.
func TestLoadTOML_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sandbox.toml")
	// Invalid TOML - missing quote
	content := `foo = bar`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTOML(path)
	if err == nil {
		t.Error("expected parse error but got none")
	}
}

// TestPathDenied tests path denial matching.
func TestPathDenied(t *testing.T) {
	eff := Effective{
		DenyGlobs: []string{"*.env", "*.log"},
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{".env", true},
		{"test.log", true},
		{"main.go", false},
		{"config.json", false},
	}

	for _, tc := range tests {
		result := eff.PathDenied(tc.path)
		if result != tc.expected {
			t.Errorf("PathDenied(%q) = %v, want %v", tc.path, result, tc.expected)
		}
	}
}

// TestPathDenied_EmptyGlobs tests with no deny globs.
func TestPathDenied_EmptyGlobs(t *testing.T) {
	eff := Effective{}
	if eff.PathDenied(".env") {
		t.Error("expected PathDenied to return false with empty globs")
	}
}

// TestThreatLevel_String_Extra tests the ThreatLevel String method.
func TestThreatLevel_String_Extra(t *testing.T) {
	tests := []struct {
		level    ThreatLevel
		expected string
	}{
		{ThreatLow, "LOW"},
		{ThreatMedium, "MEDIUM"},
		{ThreatHigh, "HIGH"},
		{ThreatCritical, "CRITICAL"},
		{ThreatLevel(999), "NONE"},
	}

	for _, tc := range tests {
		result := tc.level.String()
		if result != tc.expected {
			t.Errorf("String() = %q, want %q", result, tc.expected)
		}
	}
}

func boolPtr(b bool) *bool {
	return &b
}

// TestMergeConfigs_UserOverridesDefaultProfile tests user profile setting.
func TestMergeConfigs_UserOverridesDefaultProfile(t *testing.T) {
	user := TOMLConfig{Profile: ProfileStrict}
	project := TOMLConfig{}
	merged, err := MergeConfigs(user, project)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Profile != string(ProfileStrict) {
		t.Errorf("expected Profile 'strict', got %q", merged.Profile)
	}
}

// TestMergeConfigs_ProfileWeakeningProtection protects against weakening.
func TestMergeConfigs_ProfileWeakeningProtection(t *testing.T) {
	user := TOMLConfig{Profile: ProfileStrict}
	project := TOMLConfig{Profile: ProfileOff} // weaker than strict
	_, err := MergeConfigs(user, project)
	if err == nil {
		t.Fatal("expected error for weaker project profile")
	}
}

// TestMergeConfigs_DenyGlobsAppended appends deny globs correctly.
func TestMergeConfigs_DenyGlobsAppended(t *testing.T) {
	user := TOMLConfig{DenyGlobs: []string{"*.tmp"}}
	project := TOMLConfig{DenyGlobs: []string{"*.log"}}
	merged, err := MergeConfigs(user, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.DenyGlobs) != 2 {
		t.Errorf("expected 2 deny globs, got %d", len(merged.DenyGlobs))
	}
}

// TestResolveMode_BuiltInFallback returns ParseMode result for unknown profile names.
func TestResolveMode_BuiltInFallback(t *testing.T) {
	cfg := TOMLConfig{}
	mode := resolveMode(cfg, "devbox")
	// Unknown profile falls through to ParseMode which returns strict by default
	if mode != ModeStrict {
		t.Errorf("expected ModeStrict for unknown devbox, got %v", mode)
	}
}
