package sandbox

import (
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

// TestEffectiveFrom_Default tests default effective config.
func TestEffectiveFrom_Default(t *testing.T) {
	cfg := TOMLConfig{}
	eff, err := EffectiveFrom(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eff.Mode != ModeWorkspace {
		t.Errorf("expected ModeWorkspace, got %v", eff.Mode)
	}
	if !eff.AllowNetwork {
		t.Error("expected AllowNetwork to be true for workspace mode")
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
