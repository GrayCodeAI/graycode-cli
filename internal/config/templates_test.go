package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptTemplateApply(t *testing.T) {
	tpl := PromptTemplate{
		Name:     "test",
		Template: "Hello {{name}}, please {{action}} the {{target}}.",
		Args:     []string{"name", "action", "target"},
	}

	result := tpl.Apply(map[string]string{
		"name":   "Alice",
		"action": "review",
		"target": "code",
	})

	expected := "Hello Alice, please review the code."
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestPromptTemplateApply_MissingArgs(t *testing.T) {
	tpl := PromptTemplate{
		Name:     "test",
		Template: "Hello {{name}}, do {{action}}.",
	}

	// Missing "action" arg should leave placeholder
	result := tpl.Apply(map[string]string{"name": "Bob"})
	if result != "Hello Bob, do {{action}}." {
		t.Fatalf("expected unreplaced placeholder, got %q", result)
	}
}

func TestLoadTemplates_JSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".hawk", "templates")
	os.MkdirAll(dir, 0o755)

	tpl := PromptTemplate{
		Name:     "review",
		Template: "Review {{file}} for bugs",
		Args:     []string{"file"},
	}
	data, _ := json.Marshal(tpl)
	os.WriteFile(filepath.Join(dir, "review.json"), data, 0o644)

	templates := LoadTemplates()
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}
	if templates[0].Name != "review" {
		t.Fatalf("expected name 'review', got %q", templates[0].Name)
	}
	if templates[0].Template != "Review {{file}} for bugs" {
		t.Fatalf("unexpected template content: %q", templates[0].Template)
	}
}

func TestLoadTemplates_TextFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".hawk", "templates")
	os.MkdirAll(dir, 0o755)

	os.WriteFile(filepath.Join(dir, "explain.txt"), []byte("Explain {{concept}} simply"), 0o644)

	templates := LoadTemplates()
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}
	if templates[0].Name != "explain" {
		t.Fatalf("expected name 'explain', got %q", templates[0].Name)
	}
}

func TestLoadTemplates_EmptyDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	templates := LoadTemplates()
	if len(templates) != 0 {
		t.Fatalf("expected 0 templates from nonexistent dir, got %d", len(templates))
	}
}
