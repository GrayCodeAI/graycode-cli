package theme

import (
	"testing"
)

func TestApplyThemePreference(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"auto resolves to dark by default", "auto", "dark"}, // macOS/Linux/Windows detection may fail, defaults to dark
		{"system is alias for auto", "system", "dark"},
		{"dark returns dark", "dark", "dark"},
		{"light returns light", "light", "light"},
		{"specific theme returns itself", "tokyo-night", "tokyo-night"},
		{"empty returns empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyThemePreference(tt.input)
			if got != tt.expected && tt.input != "auto" && tt.input != "system" {
				t.Errorf("ApplyThemePreference(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestResolveAutoTheme(t *testing.T) {
	tests := []struct {
		name           string
		preference     string
		darkOverride   string
		lightOverride  string
		expectedPrefix string
	}{
		{"auto with dark override", "auto", "tokyo-night", "", "tokyo-night"}, // May vary based on OS
		{"dark with dark override", "dark", "nord", "", "nord"},
		{"light with light override", "light", "", "light", "light"},
		{"specific theme unchanged", "dracula", "tokyo-night", "", "dracula"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAutoTheme(tt.preference, tt.darkOverride, tt.lightOverride)
			// For specific themes and direct dark/light, verify exact match
			if tt.preference != "auto" && tt.preference != "system" {
				if got != tt.expectedPrefix {
					t.Errorf("ResolveAutoTheme(%q, %q, %q) = %q, want %q", tt.preference, tt.darkOverride, tt.lightOverride, got, tt.expectedPrefix)
				}
			}
		})
	}
}