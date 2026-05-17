package recipe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRecipe(t *testing.T) {
	dir := t.TempDir()
	yaml := `version: 1.0.0
title: Test Recipe
description: A test
instructions: |
  Do the thing with {{.name}}
parameters:
  - key: name
    input_type: string
    requirement: required
    description: The name
extensions:
  - type: builtin
    name: developer
activities:
  - Step 1
  - Step 2
`
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte(yaml), 0o644)

	r, err := LoadRecipe(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.Title != "Test Recipe" {
		t.Errorf("title = %q", r.Title)
	}
	if len(r.Parameters) != 1 {
		t.Errorf("params = %d", len(r.Parameters))
	}
	if len(r.Activities) != 2 {
		t.Errorf("activities = %d", len(r.Activities))
	}
}

func TestRenderPrompt(t *testing.T) {
	r := &Recipe{
		Title:        "Test",
		Instructions: "Hello {{.name}}, do {{.task}}",
		Parameters: []Parameter{
			{Key: "name", Requirement: "required"},
			{Key: "task", Default: "nothing"},
		},
	}
	got, err := r.RenderPrompt(map[string]string{"name": "World"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello World, do nothing" {
		t.Errorf("got %q", got)
	}
}

func TestRenderPrompt_MissingRequired(t *testing.T) {
	r := &Recipe{
		Title:        "Test",
		Instructions: "{{.name}}",
		Parameters:   []Parameter{{Key: "name", Requirement: "required"}},
	}
	_, err := r.RenderPrompt(map[string]string{})
	if err == nil {
		t.Error("expected error for missing required param")
	}
}

func TestRunner_List(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "r.yaml"), []byte("title: R1\ninstructions: do\n"), 0o644)
	rn := &Runner{RecipeDirs: []string{dir}}
	list := rn.List()
	if len(list) != 1 {
		t.Errorf("expected 1 recipe, got %d", len(list))
	}
}

func TestRunner_Execute(t *testing.T) {
	r := &Recipe{Title: "T", Instructions: "Do {{.x}}", Parameters: []Parameter{{Key: "x", Default: "it"}}}
	rn := NewRunner()
	got, err := rn.Execute(context.Background(), r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Do it" {
		t.Errorf("got %q", got)
	}
}
