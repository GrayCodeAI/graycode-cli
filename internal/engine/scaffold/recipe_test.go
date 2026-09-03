package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewRecipeRegistry(t *testing.T) {
	dir := t.TempDir()
	reg := NewRecipeRegistry(dir)

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	if reg.Dir != dir {
		t.Errorf("expected dir %q, got %q", dir, reg.Dir)
	}
	if reg.Recipes == nil {
		t.Fatal("expected non-nil recipes map")
	}
	if len(reg.Recipes) != 0 {
		t.Errorf("expected empty recipes, got %d", len(reg.Recipes))
	}
}

func TestRecipeCreate(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	recipe := &Recipe{
		Name:   "Test Recipe",
		Prompt: "Do something",
		Tools:  []string{"Bash"},
		Model:  "claude-sonnet-4-6",
	}

	id := reg.Create(recipe)

	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	if recipe.ID != id {
		t.Errorf("expected recipe.ID=%q, got %q", id, recipe.ID)
	}
	if reg.Recipes[id] != recipe {
		t.Error("recipe not stored in registry")
	}
	if recipe.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestCreatePreservesCreatedAt(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	recipe := &Recipe{
		Name:      "Test",
		Prompt:    "Do it",
		Tools:     []string{"Read"},
		Model:     "claude-sonnet-4-6",
		CreatedAt: ts,
	}

	reg.Create(recipe)

	if !recipe.CreatedAt.Equal(ts) {
		t.Errorf("expected CreatedAt to be preserved, got %v", recipe.CreatedAt)
	}
}

func TestEncodeDecode(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	recipe := &Recipe{
		Name:        "Fix Lint",
		Description: "Fix all lint errors",
		Prompt:      "Find and fix all lint errors in the project.",
		Tools:       []string{"Bash", "Edit", "Read"},
		Model:       "claude-sonnet-4-6",
		Provider:    "anthropic",
		Author:      "@user",
		Version:     "1.0.0",
		Settings:    map[string]interface{}{"max_tokens": float64(4096)},
	}

	// Encode
	deeplink := reg.Encode(recipe)
	if !strings.HasPrefix(deeplink, "graycode://recipe/") {
		t.Fatalf("expected graycode://recipe/ prefix, got %q", deeplink)
	}

	// Decode
	decoded, err := reg.Decode(deeplink)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if decoded.Name != recipe.Name {
		t.Errorf("name mismatch: %q vs %q", decoded.Name, recipe.Name)
	}
	if decoded.Prompt != recipe.Prompt {
		t.Errorf("prompt mismatch: %q vs %q", decoded.Prompt, recipe.Prompt)
	}
	if decoded.Model != recipe.Model {
		t.Errorf("model mismatch: %q vs %q", decoded.Model, recipe.Model)
	}
	if decoded.Provider != recipe.Provider {
		t.Errorf("provider mismatch: %q vs %q", decoded.Provider, recipe.Provider)
	}
	if decoded.Author != recipe.Author {
		t.Errorf("author mismatch: %q vs %q", decoded.Author, recipe.Author)
	}
	if len(decoded.Tools) != len(recipe.Tools) {
		t.Errorf("tools length mismatch: %d vs %d", len(decoded.Tools), len(recipe.Tools))
	}
	for i, tool := range decoded.Tools {
		if tool != recipe.Tools[i] {
			t.Errorf("tool[%d] mismatch: %q vs %q", i, tool, recipe.Tools[i])
		}
	}
}

func TestDecodeInvalidPrefix(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())

	_, err := reg.Decode("https://example.com/recipe/abc")
	if err == nil {
		t.Fatal("expected error for invalid prefix")
	}
	if !strings.Contains(err.Error(), "must start with graycode://recipe/") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDecodeInvalidBase64(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())

	_, err := reg.Decode("graycode://recipe/!!!invalid!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "invalid deeplink encoding") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDecodeInvalidJSON(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())

	// Valid base64 but invalid JSON
	_, err := reg.Decode("graycode://recipe/bm90LWpzb24=")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "invalid recipe payload") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRecipeExecute(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	recipe := &Recipe{
		Name:     "Test",
		Prompt:   "hello world",
		Tools:    []string{"Bash"},
		Model:    "claude-sonnet-4-6",
		Provider: "anthropic",
		Settings: map[string]interface{}{"temperature": 0.5},
	}

	var capturedCtx context.Context
	var capturedPrompt string
	execFn := func(ctx context.Context, prompt string) (string, error) {
		capturedCtx = ctx
		capturedPrompt = prompt
		return "done", nil
	}

	result, err := reg.Execute(context.Background(), recipe, execFn)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if result != "done" {
		t.Errorf("expected result %q, got %q", "done", result)
	}
	if capturedPrompt != "hello world" {
		t.Errorf("expected prompt %q, got %q", "hello world", capturedPrompt)
	}

	// Verify context values
	if capturedCtx.Value(recipeCtxKey("model")) != "claude-sonnet-4-6" {
		t.Error("model not set in context")
	}
	if capturedCtx.Value(recipeCtxKey("provider")) != "anthropic" {
		t.Error("provider not set in context")
	}
	tools := capturedCtx.Value(recipeCtxKey("tools")).([]string)
	if len(tools) != 1 || tools[0] != "Bash" {
		t.Errorf("tools not set correctly in context: %v", tools)
	}
}

func TestExecuteNilRecipe(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	_, err := reg.Execute(context.Background(), nil, func(ctx context.Context, s string) (string, error) {
		return "", nil
	})
	if err == nil {
		t.Fatal("expected error for nil recipe")
	}
}

func TestExecuteNilFn(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	recipe := &Recipe{Name: "test", Prompt: "p", Tools: []string{"Bash"}, Model: "m"}
	_, err := reg.Execute(context.Background(), recipe, nil)
	if err == nil {
		t.Fatal("expected error for nil execFn")
	}
}

func TestRecipeList(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())

	// Empty list
	if len(reg.List()) != 0 {
		t.Errorf("expected empty list, got %d", len(reg.List()))
	}

	// Add recipes
	reg.Create(&Recipe{Name: "A", Prompt: "a", Tools: []string{"Bash"}, Model: "m"})
	reg.Create(&Recipe{Name: "B", Prompt: "b", Tools: []string{"Edit"}, Model: "m"})

	list := reg.List()
	if len(list) != 2 {
		t.Errorf("expected 2 recipes, got %d", len(list))
	}
}

func TestRecipeGet(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	recipe := &Recipe{Name: "Test", Prompt: "do stuff", Tools: []string{"Bash"}, Model: "m"}
	id := reg.Create(recipe)

	got := reg.Get(id)
	if got != recipe {
		t.Error("expected to get the same recipe back")
	}

	// Non-existent
	if reg.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestShare(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	recipe := &Recipe{
		Name:        "Full Recipe",
		Description: "Long description that should be omitted",
		Prompt:      "do it",
		Tools:       []string{"Bash", "Edit"},
		Model:       "claude-sonnet-4-6",
		Provider:    "anthropic",
		Author:      "@dev",
		Version:     "2.0.0",
		CreatedAt:   time.Now(),
	}

	shareURL := reg.Share(recipe)
	if !strings.HasPrefix(shareURL, "graycode://recipe/") {
		t.Fatalf("expected graycode://recipe/ prefix, got %q", shareURL)
	}

	// Decode and verify minimal fields are present
	decoded, err := reg.Decode(shareURL)
	if err != nil {
		t.Fatalf("decode share URL failed: %v", err)
	}
	if decoded.Name != recipe.Name {
		t.Errorf("name mismatch: %q", decoded.Name)
	}
	if decoded.Prompt != recipe.Prompt {
		t.Errorf("prompt mismatch: %q", decoded.Prompt)
	}
	if decoded.Model != recipe.Model {
		t.Errorf("model mismatch: %q", decoded.Model)
	}

	// Verify omitted fields
	if decoded.Description != "" {
		t.Errorf("description should be omitted in share URL, got %q", decoded.Description)
	}
	if decoded.Version != "" {
		t.Errorf("version should be omitted in share URL, got %q", decoded.Version)
	}
	if !decoded.CreatedAt.IsZero() {
		t.Errorf("createdAt should be omitted in share URL, got %v", decoded.CreatedAt)
	}
}

func TestImportFromURL(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	original := &Recipe{
		Name:   "Import Me",
		Prompt: "do import",
		Tools:  []string{"Read"},
		Model:  "claude-sonnet-4-6",
	}

	deeplink := reg.Encode(original)

	imported, err := reg.ImportFromURL(deeplink)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if imported.Name != original.Name {
		t.Errorf("name mismatch: %q vs %q", imported.Name, original.Name)
	}
	if imported.ID == "" {
		t.Error("imported recipe should have an ID")
	}

	// Should be in registry
	got := reg.Get(imported.ID)
	if got == nil {
		t.Error("imported recipe not found in registry")
	}
}

func TestImportFromURLInvalid(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())

	_, err := reg.ImportFromURL("https://bad-url.com/recipe/abc")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "import failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFormatRecipe(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	recipe := &Recipe{
		Name:   "Fix all lint errors",
		Prompt: "Find and fix all lint errors in the project.\nRun all linters, fix each issue, then verify clean.",
		Tools:  []string{"Bash", "Edit", "Read"},
		Model:  "claude-sonnet-4-6",
		Author: "@user",
	}

	formatted := reg.FormatRecipe(recipe)

	if !strings.Contains(formatted, `Recipe: "Fix all lint errors"`) {
		t.Error("missing recipe name in formatted output")
	}
	if !strings.Contains(formatted, "Author: @user") {
		t.Error("missing author in formatted output")
	}
	if !strings.Contains(formatted, "Model: claude-sonnet-4-6") {
		t.Error("missing model in formatted output")
	}
	if !strings.Contains(formatted, "Tools: [Bash, Edit, Read]") {
		t.Error("missing tools in formatted output")
	}
	if !strings.Contains(formatted, "Prompt: Find and fix all lint errors") {
		t.Error("missing prompt in formatted output")
	}
	if !strings.Contains(formatted, "Share: graycode://recipe/") {
		t.Error("missing share link in formatted output")
	}
}

func TestRecipeSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	reg := NewRecipeRegistry(dir)

	reg.Create(&Recipe{
		Name:   "Recipe A",
		Prompt: "do A",
		Tools:  []string{"Bash"},
		Model:  "model-a",
		Author: "@alice",
	})
	reg.Create(&Recipe{
		Name:   "Recipe B",
		Prompt: "do B",
		Tools:  []string{"Edit", "Read"},
		Model:  "model-b",
		Author: "@bob",
	})

	// Save
	if err := reg.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "recipes.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("recipes.json not created")
	}

	// Load into new registry
	reg2 := NewRecipeRegistry(dir)
	if err := reg2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if len(reg2.Recipes) != 2 {
		t.Errorf("expected 2 recipes after load, got %d", len(reg2.Recipes))
	}

	// Verify content
	for id, recipe := range reg.Recipes {
		loaded := reg2.Recipes[id]
		if loaded == nil {
			t.Errorf("recipe %q not found after load", id)
			continue
		}
		if loaded.Name != recipe.Name {
			t.Errorf("name mismatch for %q: %q vs %q", id, loaded.Name, recipe.Name)
		}
		if loaded.Prompt != recipe.Prompt {
			t.Errorf("prompt mismatch for %q", id)
		}
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	reg := NewRecipeRegistry(dir)

	// Loading from a dir with no recipes.json should not error
	if err := reg.Load(); err != nil {
		t.Fatalf("load should succeed with no file: %v", err)
	}
	if len(reg.Recipes) != 0 {
		t.Errorf("expected empty recipes, got %d", len(reg.Recipes))
	}
}

func TestRecipeSaveNoDir(t *testing.T) {
	reg := NewRecipeRegistry("")
	if err := reg.Save(); err == nil {
		t.Fatal("expected error when dir is empty")
	}
}

func TestRecipeLoadNoDir(t *testing.T) {
	reg := NewRecipeRegistry("")
	if err := reg.Load(); err == nil {
		t.Fatal("expected error when dir is empty")
	}
}

func TestRecipeValidate(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())

	tests := []struct {
		name     string
		recipe   *Recipe
		wantErrs int
		contains []string
	}{
		{
			name:     "nil recipe",
			recipe:   nil,
			wantErrs: 1,
			contains: []string{"recipe is nil"},
		},
		{
			name:     "valid recipe",
			recipe:   &Recipe{Name: "Test", Prompt: "do it", Tools: []string{"Bash"}, Model: "m"},
			wantErrs: 0,
		},
		{
			name:     "missing all required fields",
			recipe:   &Recipe{},
			wantErrs: 4,
			contains: []string{"name is required", "prompt is required", "at least one tool is required", "model is required"},
		},
		{
			name:     "invalid version",
			recipe:   &Recipe{Name: "Test", Prompt: "do", Tools: []string{"Bash"}, Model: "m", Version: "bad"},
			wantErrs: 1,
			contains: []string{"version must be"},
		},
		{
			name:     "valid semver",
			recipe:   &Recipe{Name: "Test", Prompt: "do", Tools: []string{"Bash"}, Model: "m", Version: "1.2.3"},
			wantErrs: 0,
		},
		{
			name:     "valid two-part version",
			recipe:   &Recipe{Name: "Test", Prompt: "do", Tools: []string{"Bash"}, Model: "m", Version: "1.0"},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := reg.Validate(tt.recipe)
			if len(errs) != tt.wantErrs {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErrs, len(errs), errs)
			}
			for _, want := range tt.contains {
				found := false
				for _, e := range errs {
					if strings.Contains(e, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error containing %q, got %v", want, errs)
				}
			}
		})
	}
}

func TestRecipeConcurrentAccess(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())
	var wg sync.WaitGroup

	// Concurrent creates
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			recipe := &Recipe{
				Name:   strings.Repeat("x", n+1),
				Prompt: "do",
				Tools:  []string{"Bash"},
				Model:  "m",
			}
			reg.Create(recipe)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			reg.List()
		}()
	}

	wg.Wait()

	if len(reg.Recipes) != 50 {
		t.Errorf("expected 50 recipes, got %d", len(reg.Recipes))
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	reg := NewRecipeRegistry(t.TempDir())

	// Test with special characters
	recipe := &Recipe{
		Name:   "Recipe with \"quotes\" & <special> chars",
		Prompt: "Handle\nnewlines\tand\ttabs",
		Tools:  []string{"Bash"},
		Model:  "claude-sonnet-4-6",
		Settings: map[string]interface{}{
			"nested": map[string]interface{}{
				"key": "value",
			},
		},
	}

	deeplink := reg.Encode(recipe)
	decoded, err := reg.Decode(deeplink)
	if err != nil {
		t.Fatalf("round trip decode failed: %v", err)
	}

	if decoded.Name != recipe.Name {
		t.Errorf("name mismatch after round trip: %q vs %q", decoded.Name, recipe.Name)
	}
	if decoded.Prompt != recipe.Prompt {
		t.Errorf("prompt mismatch after round trip")
	}
}

func TestGenerateRecipeIDDeterministic(t *testing.T) {
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	recipe := &Recipe{
		Name:      "Test",
		Prompt:    "do it",
		CreatedAt: ts,
	}

	id1 := generateRecipeID(recipe)
	id2 := generateRecipeID(recipe)

	if id1 != id2 {
		t.Errorf("expected deterministic ID, got %q and %q", id1, id2)
	}
	if len(id1) != 16 {
		t.Errorf("expected 16 char ID, got %d", len(id1))
	}
}
