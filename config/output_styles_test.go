package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOutputStyles_Empty(t *testing.T) {
	orig := os.Getenv("HOME")
	tmp := t.TempDir()
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)

	// No style directories exist
	styles := LoadOutputStyles()
	if len(styles) != 0 {
		t.Fatalf("expected 0 styles, got %d", len(styles))
	}
}

func TestLoadOutputStyles_WithFiles(t *testing.T) {
	tmp := t.TempDir()
	stylesDir := filepath.Join(tmp, ".hawk", "output-styles")
	os.MkdirAll(stylesDir, 0o755)

	// Create a concise style
	os.WriteFile(filepath.Join(stylesDir, "concise.md"), []byte("# Be concise\nRespond in bullet points.\n\n{{content}}"), 0o644)

	// Create a verbose style with keep-instructions marker
	os.WriteFile(filepath.Join(stylesDir, "verbose.md"), []byte("# Detailed explanation\n<!-- keep-instructions -->\n{{content}}"), 0o644)

	orig := os.Getenv("HOME")
	os.Setenv("HOME", tmp)
	defer os.Setenv("HOME", orig)

	styles := LoadOutputStyles()
	if len(styles) != 2 {
		t.Fatalf("expected 2 styles, got %d", len(styles))
	}

	found := false
	for _, s := range styles {
		if s.Name == "verbose" && s.KeepInstructions {
			found = true
		}
	}
	if !found {
		t.Fatal("expected verbose style with KeepInstructions=true")
	}
}

func TestApplyOutputStyle_WithPlaceholder(t *testing.T) {
	style := OutputStyle{
		Name:     "test",
		Template: "PREFIX\n{{content}}\nSUFFIX",
	}
	result := ApplyOutputStyle("hello world", style)
	expected := "PREFIX\nhello world\nSUFFIX"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestApplyOutputStyle_WithoutPlaceholder(t *testing.T) {
	style := OutputStyle{
		Name:     "test",
		Template: "Be concise.",
	}
	result := ApplyOutputStyle("hello world", style)
	expected := "Be concise.\n\nhello world"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}
