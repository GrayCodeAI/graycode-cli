package scaffold

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Recipe represents a shareable task recipe that captures instructions,
// tools, settings, and metadata for one-click execution via deeplinks.
type Recipe struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Prompt      string                 `json:"prompt"`
	Tools       []string               `json:"tools"`
	Model       string                 `json:"model"`
	Provider    string                 `json:"provider"`
	Settings    map[string]interface{} `json:"settings,omitempty"`
	Author      string                 `json:"author"`
	Version     string                 `json:"version"`
	CreatedAt   time.Time              `json:"created_at"`
}

// RecipeRegistry manages a collection of recipes with thread-safe access.
type RecipeRegistry struct {
	Recipes map[string]*Recipe
	Dir     string
	mu      sync.RWMutex
}

// NewRecipeRegistry creates a new RecipeRegistry that stores recipes in dir.
func NewRecipeRegistry(dir string) *RecipeRegistry {
	return &RecipeRegistry{
		Recipes: make(map[string]*Recipe),
		Dir:     dir,
	}
}

// Create adds a recipe to the registry and returns its generated ID.
func (r *RecipeRegistry) Create(recipe *Recipe) string {
	r.mu.Lock()
	defer r.mu.Unlock()

	if recipe.CreatedAt.IsZero() {
		recipe.CreatedAt = time.Now()
	}

	id := generateRecipeID(recipe)
	recipe.ID = id
	r.Recipes[id] = recipe
	return id
}

// Encode serializes a recipe to JSON, base64url-encodes it, and returns
// a graycode:// deeplink URL.
func (r *RecipeRegistry) Encode(recipe *Recipe) string {
	data, _ := json.Marshal(recipe)
	encoded := base64.URLEncoding.EncodeToString(data)
	return "graycode://recipe/" + encoded
}

// Decode parses a graycode:// deeplink URL, base64url-decodes the payload,
// and deserializes it into a Recipe.
func (r *RecipeRegistry) Decode(deeplink string) (*Recipe, error) {
	const prefix = "graycode://recipe/"
	if !strings.HasPrefix(deeplink, prefix) {
		return nil, errors.New("invalid deeplink: must start with graycode://recipe/")
	}

	encoded := strings.TrimPrefix(deeplink, prefix)
	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid deeplink encoding: %w", err)
	}

	var recipe Recipe
	if err := json.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("invalid recipe payload: %w", err)
	}

	return &recipe, nil
}

// Execute applies the recipe settings and runs the prompt using the provided
// execution function. The execFn receives the context and the recipe prompt,
// and returns the output or an error.
func (r *RecipeRegistry) Execute(ctx context.Context, recipe *Recipe, execFn func(context.Context, string) (string, error)) (string, error) {
	if recipe == nil {
		return "", errors.New("recipe is nil")
	}
	if execFn == nil {
		return "", errors.New("execution function is nil")
	}

	// Apply settings to context via values
	execCtx := ctx
	if recipe.Model != "" {
		execCtx = context.WithValue(execCtx, recipeCtxKey("model"), recipe.Model)
	}
	if recipe.Provider != "" {
		execCtx = context.WithValue(execCtx, recipeCtxKey("provider"), recipe.Provider)
	}
	if len(recipe.Tools) > 0 {
		execCtx = context.WithValue(execCtx, recipeCtxKey("tools"), recipe.Tools)
	}
	if len(recipe.Settings) > 0 {
		execCtx = context.WithValue(execCtx, recipeCtxKey("settings"), recipe.Settings)
	}

	return execFn(execCtx, recipe.Prompt)
}

// List returns all recipes in the registry.
func (r *RecipeRegistry) List() []*Recipe {
	r.mu.RLock()
	defer r.mu.RUnlock()

	recipes := make([]*Recipe, 0, len(r.Recipes))
	for _, recipe := range r.Recipes {
		recipes = append(recipes, recipe)
	}
	return recipes
}

// Get returns a recipe by ID, or nil if not found.
func (r *RecipeRegistry) Get(id string) *Recipe {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Recipes[id]
}

// Share generates a compact shareable deeplink URL for the recipe.
func (r *RecipeRegistry) Share(recipe *Recipe) string {
	// Create a minimal copy to reduce URL size
	minimal := &Recipe{
		Name:   recipe.Name,
		Prompt: recipe.Prompt,
		Tools:  recipe.Tools,
		Model:  recipe.Model,
	}
	if recipe.Provider != "" {
		minimal.Provider = recipe.Provider
	}
	if recipe.Author != "" {
		minimal.Author = recipe.Author
	}
	if len(recipe.Settings) > 0 {
		minimal.Settings = recipe.Settings
	}

	return r.Encode(minimal)
}

// ImportFromURL decodes a graycode:// deeplink URL and imports the recipe into
// the registry.
func (r *RecipeRegistry) ImportFromURL(url string) (*Recipe, error) {
	recipe, err := r.Decode(url)
	if err != nil {
		return nil, fmt.Errorf("import failed: %w", err)
	}

	r.Create(recipe)
	return recipe, nil
}

// FormatRecipe returns a human-readable formatted string representation
// of the recipe.
func (r *RecipeRegistry) FormatRecipe(recipe *Recipe) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Recipe: %q\n", recipe.Name))

	// Author and model line
	parts := []string{}
	if recipe.Author != "" {
		parts = append(parts, fmt.Sprintf("Author: %s", recipe.Author))
	}
	if recipe.Model != "" {
		parts = append(parts, fmt.Sprintf("Model: %s", recipe.Model))
	}
	if len(parts) > 0 {
		b.WriteString(strings.Join(parts, " | ") + "\n")
	}

	// Tools
	if len(recipe.Tools) > 0 {
		b.WriteString(fmt.Sprintf("Tools: [%s]\n", strings.Join(recipe.Tools, ", ")))
	}

	// Prompt
	b.WriteString(fmt.Sprintf("\nPrompt: %s\n", recipe.Prompt))

	// Share link
	shareURL := r.Share(recipe)
	b.WriteString(fmt.Sprintf("\nShare: %s\n", shareURL))

	return b.String()
}

// Save persists all recipes in the registry to disk as a JSON file.
func (r *RecipeRegistry) Save() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.Dir == "" {
		return errors.New("registry directory not set")
	}

	if err := os.MkdirAll(r.Dir, 0o750); err != nil {
		return fmt.Errorf("failed to create recipe directory: %w", err)
	}

	data, err := json.MarshalIndent(r.Recipes, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal recipes: %w", err)
	}

	path := filepath.Join(r.Dir, "recipes.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write recipes file: %w", err)
	}

	return nil
}

// Load reads recipes from the registry's directory on disk.
func (r *RecipeRegistry) Load() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Dir == "" {
		return errors.New("registry directory not set")
	}

	path := filepath.Join(r.Dir, "recipes.json")
	data, err := os.ReadFile(path) // #nosec G304 -- path provided by caller via tool/task parameters, inherent to this dev CLI's file operations
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No recipes file yet is fine
		}
		return fmt.Errorf("failed to read recipes file: %w", err)
	}

	recipes := make(map[string]*Recipe)
	if err := json.Unmarshal(data, &recipes); err != nil {
		return fmt.Errorf("failed to unmarshal recipes: %w", err)
	}

	r.Recipes = recipes
	return nil
}

// Validate checks a recipe for required fields and returns a list of
// validation error messages. An empty slice means the recipe is valid.
func (r *RecipeRegistry) Validate(recipe *Recipe) []string {
	var errs []string

	if recipe == nil {
		return []string{"recipe is nil"}
	}

	if strings.TrimSpace(recipe.Name) == "" {
		errs = append(errs, "name is required")
	}

	if strings.TrimSpace(recipe.Prompt) == "" {
		errs = append(errs, "prompt is required")
	}

	if len(recipe.Tools) == 0 {
		errs = append(errs, "at least one tool is required")
	}

	if recipe.Model == "" {
		errs = append(errs, "model is required")
	}

	if recipe.Version != "" {
		// Validate version format (semver-like: major.minor.patch)
		parts := strings.Split(recipe.Version, ".")
		if len(parts) < 2 || len(parts) > 3 {
			errs = append(errs, "version must be in format major.minor or major.minor.patch")
		}
	}

	return errs
}

// recipeCtxKey is a context key type for recipe execution values.
type recipeCtxKey string

// generateRecipeID creates a deterministic ID from the recipe content.
func generateRecipeID(recipe *Recipe) string {
	h := sha256.New()
	h.Write([]byte(recipe.Name))
	h.Write([]byte(recipe.Prompt))
	h.Write([]byte(recipe.CreatedAt.Format(time.RFC3339Nano)))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
