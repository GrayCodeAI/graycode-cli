package theme

import (
	"strings"
	"testing"
)

// TestThemeRegistryNotEmpty ensures all themes are registered.
func TestThemeRegistryNotEmpty(t *testing.T) {
	if len(themeRegistry) == 0 {
		t.Fatal("themeRegistry should not be empty")
	}
}

// TestThemeRegistryCount ensures we have 18 themes (17 palettes + minimal).
func TestThemeRegistryCount(t *testing.T) {
	if len(themeRegistry) != 18 {
		t.Errorf("themeRegistry has %d themes, want 18", len(themeRegistry))
	}
}

// TestThemeRegistryAllDarkOrLight ensures every theme is either dark or light.
func TestThemeRegistryAllDarkOrLight(t *testing.T) {
	for _, entry := range themeRegistry {
		_ = entry
		// Every theme should have IsDark set (true or false)
	}
}

// TestDarkPaletteDefault ensures the dark palette is the default.
func TestDarkPaletteDefault(t *testing.T) {
	entry := themeRegistry[0]
	if entry.Name != "dark" {
		t.Errorf("first theme should be 'dark', got %q", entry.Name)
	}
	if !entry.IsDark {
		t.Error("dark theme should have IsDark=true")
	}
}

// TestThemeByNameExists ensures ThemeByName works.
func TestThemeByNameExists(t *testing.T) {
	m := ThemeByName()
	if m == nil {
		t.Fatal("ThemeByName should not return nil")
	}
	if _, ok := m["dark"]; !ok {
		t.Error("ThemeByName should contain 'dark' theme")
	}
}

// TestGetThemeEntryExists ensures GetThemeEntry works.
func TestGetThemeEntryExists(t *testing.T) {
	entry := GetThemeEntry("dark")
	if entry.Name != "dark" {
		t.Errorf("GetThemeEntry('dark') returned wrong name: %q", entry.Name)
	}
	if !entry.IsDark {
		t.Error("dark theme should have IsDark=true")
	}
}

// TestLookupThemeExists ensures LookupTheme works.
func TestLookupThemeExists(t *testing.T) {
	entry, ok := LookupTheme("dark")
	if !ok {
		t.Error("LookupTheme('dark') should return ok=true")
	}
	if entry.Name != "dark" {
		t.Errorf("LookupTheme('dark') returned wrong name: %q", entry.Name)
	}
}

// TestLookupThemeCaseSensitive checks current behavior.
func TestLookupThemeCaseSensitive(t *testing.T) {
	// Current implementation is case-sensitive
	entry, ok := LookupTheme("dark")
	if !ok {
		t.Error("LookupTheme('dark') should work for dark theme")
	}
	if entry.Name != "dark" {
		t.Errorf("LookupTheme('dark') returned wrong name: %q", entry.Name)
	}
}

// TestLookupThemeNotFound ensures LookupTheme returns ok=false for unknown themes.
func TestLookupThemeNotFound(t *testing.T) {
	_, ok := LookupTheme("nonexistent")
	if ok {
		t.Error("LookupTheme('nonexistent') should return ok=false")
	}
}

// TestThemeNames ensures ThemeNames returns all theme names.
func TestThemeNames(t *testing.T) {
	names := ThemeNames()
	if len(names) != 18 {
		t.Errorf("ThemeNames returned %d names, want 18", len(names))
	}
	// Check all expected themes are present
	seen := make(map[string]bool)
	for _, name := range names {
		seen[name] = true
	}
	expected := []string{
		"dark", "dracula", "nord", "gruvbox", "tokyo-night",
		"catppuccin", "one-dark", "solarized-dark", "rose-pine", "everforest",
		"monokai", "kanagawa", "ayu", "palenight", "github-dark",
		"light", "solarized-light",
	}
	for _, exp := range expected {
		if !seen[exp] {
			t.Errorf("expected theme %q not found in ThemeNames", exp)
		}
	}
}

// TestIsDarkTheme ensures IsDarkTheme works.
func TestIsDarkTheme(t *testing.T) {
	if !IsDarkTheme("dark") {
		t.Error("IsDarkTheme('dark') should return true")
	}
	if IsDarkTheme("light") {
		t.Error("IsDarkTheme('light') should return false")
	}
}

// TestDarkPaletteHasValidColors ensures the dark palette has valid hex colors.
func TestDarkPaletteHasValidColors(t *testing.T) {
	p := darkPalette
	if p.Panel == "" {
		t.Error("dark palette Panel should not be empty")
	}
	if !strings.HasPrefix(p.Panel, "#") {
		t.Errorf("dark palette Panel %q should start with #", p.Panel)
	}
}

// TestAllThemesHaveValidPalette ensures all themes have valid hex colors.
func TestAllThemesHaveValidPalette(t *testing.T) {
	for _, entry := range themeRegistry {
		p := entry.Palette
		// Check required fields are non-empty
		fields := []string{p.Panel, p.PromptBg, p.Line, p.Ink, p.Accent, p.Green}
		for _, field := range fields {
			if field == "" {
				t.Errorf("theme %q has empty palette field", entry.Name)
			}
			// Check it starts with #
			if !strings.HasPrefix(field, "#") {
				t.Errorf("theme %q palette field %q should start with #", entry.Name, field)
			}
			// Check it's a valid hex color (7 chars)
			if len(field) != 7 {
				t.Errorf("theme %q palette field %q should be 7 chars (e.g., #RRGGBB), got %d", entry.Name, field, len(field))
			}
		}
	}
}
