// Package recipe implements a YAML-based guided workflow system.
// Recipes are declarative multi-step tasks with parameters, extensions, and activities.
package recipe

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
"github.com/GrayCodeAI/hawk/internal/home"
)

// Recipe is the top-level YAML recipe definition.
type Recipe struct {
	Version      string      `yaml:"version"`
	Title        string      `yaml:"title"`
	Description  string      `yaml:"description"`
	Author       Author      `yaml:"author"`
	Instructions string      `yaml:"instructions"`
	Parameters   []Parameter `yaml:"parameters"`
	Extensions   []Extension `yaml:"extensions"`
	Activities   []string    `yaml:"activities"`
	Prompt       string      `yaml:"prompt"`
	SubRecipes   []string    `yaml:"sub_recipes"`
}

// Author identifies the recipe creator.
type Author struct {
	Contact string `yaml:"contact"`
}

// Parameter is a configurable input to a recipe.
type Parameter struct {
	Key         string `yaml:"key"`
	InputType   string `yaml:"input_type"`
	Requirement string `yaml:"requirement"`
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Value       string `yaml:"value"`
}

// Extension declares a tool/extension needed by the recipe.
type Extension struct {
	Type string `yaml:"type"`
	Name string `yaml:"name"`
}

// LoadRecipe reads and parses a YAML recipe file.
func LoadRecipe(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recipe: %w", err)
	}
	var r Recipe
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse recipe: %w", err)
	}
	if r.Title == "" {
		return nil, fmt.Errorf("recipe missing title")
	}
	return &r, nil
}

// LoadRecipesFromDir loads all .yaml/.yml recipes from a directory.
func LoadRecipesFromDir(dir string) ([]*Recipe, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var recipes []*Recipe
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		r, err := LoadRecipe(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		recipes = append(recipes, r)
	}
	return recipes, nil
}

// RenderPrompt applies parameter values to the recipe prompt template.
func (r *Recipe) RenderPrompt(params map[string]string) (string, error) {
	// Merge defaults with provided params
	merged := make(map[string]string)
	for _, p := range r.Parameters {
		if p.Default != "" {
			merged[p.Key] = p.Default
		}
		if p.Value != "" {
			merged[p.Key] = p.Value
		}
	}
	for k, v := range params {
		merged[k] = v
	}

	// Check required params
	for _, p := range r.Parameters {
		if p.Requirement == "required" {
			if _, ok := merged[p.Key]; !ok {
				return "", fmt.Errorf("missing required parameter: %s", p.Key)
			}
		}
	}

	// Render template
	prompt := r.Prompt
	if prompt == "" {
		prompt = r.Instructions
	}
	tmpl, err := template.New("recipe").Parse(prompt)
	if err != nil {
		return "", fmt.Errorf("template parse: %w", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, merged); err != nil {
		return "", fmt.Errorf("template exec: %w", err)
	}
	return buf.String(), nil
}

// Validate checks a recipe for completeness.
func (r *Recipe) Validate() error {
	if r.Title == "" {
		return fmt.Errorf("missing title")
	}
	if r.Instructions == "" && r.Prompt == "" {
		return fmt.Errorf("missing instructions or prompt")
	}
	return nil
}

// Runner executes recipes.
type Runner struct {
	RecipeDirs []string
	Timeout    time.Duration
}

// NewRunner creates a recipe runner with default directories.
func NewRunner() *Runner {
	home := home.Dir()
	return &Runner{
		RecipeDirs: []string{
			filepath.Join(home, ".hawk", "recipes"),
			".hawk/recipes",
		},
		Timeout: 30 * time.Minute,
	}
}

// List returns all available recipes.
func (rn *Runner) List() []*Recipe {
	var all []*Recipe
	for _, dir := range rn.RecipeDirs {
		recipes, _ := LoadRecipesFromDir(dir)
		all = append(all, recipes...)
	}
	return all
}

// Execute runs a recipe with the given parameters, returning the rendered prompt.
func (rn *Runner) Execute(_ context.Context, r *Recipe, params map[string]string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return r.RenderPrompt(params)
}
