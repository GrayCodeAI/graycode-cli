package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAliases(t *testing.T) {
	aliases := DefaultAliases()
	if len(aliases) != 3 {
		t.Fatalf("expected 3 default aliases, got %d", len(aliases))
	}
	if aliases["fix"] != "Find and fix the bug in" {
		t.Fatalf("unexpected fix alias: %q", aliases["fix"])
	}
	if aliases["test"] != "Write tests for" {
		t.Fatalf("unexpected test alias: %q", aliases["test"])
	}
	if aliases["review"] != "Review this code for issues:" {
		t.Fatalf("unexpected review alias: %q", aliases["review"])
	}
}

func TestLoadAliases_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	aliases := LoadAliases()
	if len(aliases) != 3 {
		t.Fatalf("expected defaults when no file, got %d entries", len(aliases))
	}
}

func TestSaveAndLoadAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	custom := map[string]string{
		"fix":    "Find and fix the bug in",
		"test":   "Write tests for",
		"review": "Review this code for issues:",
		"deploy": "Deploy this to production",
	}
	if err := SaveAliases(custom); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded := LoadAliases()
	if loaded["deploy"] != "Deploy this to production" {
		t.Fatalf("custom alias not loaded: %v", loaded)
	}
	// Defaults should still be present
	if loaded["fix"] != "Find and fix the bug in" {
		t.Fatalf("default alias missing: %v", loaded)
	}
}

func TestLoadAliases_MergesDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Save a file with only one alias
	os.MkdirAll(filepath.Join(home, ".hawk"), 0o755)
	os.WriteFile(filepath.Join(home, ".hawk", "aliases.json"), []byte(`{"deploy":"Ship it"}`), 0o644)

	loaded := LoadAliases()
	if loaded["deploy"] != "Ship it" {
		t.Fatalf("custom alias missing: %v", loaded)
	}
	if loaded["fix"] != "Find and fix the bug in" {
		t.Fatalf("default alias not merged: %v", loaded)
	}
}

func TestResolveAlias_Match(t *testing.T) {
	aliases := map[string]string{
		"fix":  "Find and fix the bug in",
		"test": "Write tests for",
	}
	tests := []struct {
		input    string
		expected string
	}{
		{"fix main.go", "Find and fix the bug in main.go"},
		{"test utils.go", "Write tests for utils.go"},
		{"fix", "Find and fix the bug in"},
		{"unknown command", "unknown command"},
		{"", ""},
	}
	for _, tt := range tests {
		got := ResolveAlias(tt.input, aliases)
		if got != tt.expected {
			t.Errorf("ResolveAlias(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveAlias_NoMatch(t *testing.T) {
	aliases := map[string]string{"fix": "Find and fix the bug in"}
	input := "build the project"
	got := ResolveAlias(input, aliases)
	if got != input {
		t.Fatalf("expected unchanged input, got %q", got)
	}
}
