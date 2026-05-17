package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNewModelPackRegistry(t *testing.T) {
	r := NewModelPackRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if r.ActivePack != "default" {
		t.Errorf("expected active pack 'default', got %q", r.ActivePack)
	}

	expectedPacks := []string{"default", "budget", "quality", "speed", "local", "balanced"}
	for _, name := range expectedPacks {
		if _, ok := r.Packs[name]; !ok {
			t.Errorf("expected built-in pack %q to exist", name)
		}
	}
}

func TestGetModel(t *testing.T) {
	r := NewModelPackRegistry()

	tests := []struct {
		role      string
		wantModel string
	}{
		{"code", "claude-sonnet-4-6"},
		{"summarize", "claude-haiku-4-5"},
		{"plan", "claude-opus-4-6"},
		{"debug", "claude-opus-4-6"},
		{"chat", "claude-sonnet-4-6"},
		{"review", "claude-sonnet-4-6"},
	}

	for _, tt := range tests {
		mr := r.GetModel(tt.role)
		if mr.Model != tt.wantModel {
			t.Errorf("GetModel(%q): got model %q, want %q", tt.role, mr.Model, tt.wantModel)
		}
	}
}

func TestGetModel_UnknownRole(t *testing.T) {
	r := NewModelPackRegistry()
	mr := r.GetModel("nonexistent")
	if mr.Model != "" {
		t.Errorf("expected empty model for unknown role, got %q", mr.Model)
	}
}

func TestGetModel_UnknownPack(t *testing.T) {
	r := NewModelPackRegistry()
	r.ActivePack = "does-not-exist"
	mr := r.GetModel("code")
	if mr.Model != "" {
		t.Errorf("expected empty model for unknown pack, got %q", mr.Model)
	}
}

func TestSetActive(t *testing.T) {
	r := NewModelPackRegistry()

	if err := r.SetActive("budget"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.ActivePack != "budget" {
		t.Errorf("expected active pack 'budget', got %q", r.ActivePack)
	}

	// Verify GetModel now uses the budget pack.
	mr := r.GetModel("code")
	if mr.Model != "claude-haiku-4-5" {
		t.Errorf("expected haiku for code in budget pack, got %q", mr.Model)
	}
}

func TestSetActive_NotFound(t *testing.T) {
	r := NewModelPackRegistry()
	err := r.SetActive("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent pack")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention pack name: %v", err)
	}
}

func TestRegister(t *testing.T) {
	r := NewModelPackRegistry()

	custom := &ModelPack{
		Name:        "custom",
		Description: "A custom pack for testing",
		Models: map[string]ModelRole{
			"code": {Provider: "openai", Model: "gpt-4o", Temperature: 0.1, MaxTokens: 4096, Purpose: "code"},
			"chat": {Provider: "openai", Model: "gpt-4o-mini", Temperature: 0.8, MaxTokens: 2048, Purpose: "chat"},
		},
		DefaultProvider: "openai",
		Settings:        map[string]interface{}{"api_version": "2024-01"},
		Tags:            []string{"custom", "openai"},
		Author:          "tester",
	}

	r.Register(custom)

	if _, ok := r.Packs["custom"]; !ok {
		t.Fatal("expected custom pack to be registered")
	}

	// Switch to it and verify.
	if err := r.SetActive("custom"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mr := r.GetModel("code")
	if mr.Model != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %q", mr.Model)
	}
}

func TestRegister_Overwrite(t *testing.T) {
	r := NewModelPackRegistry()

	override := &ModelPack{
		Name:        "budget",
		Description: "Overridden budget pack",
		Models: map[string]ModelRole{
			"code": {Provider: "anthropic", Model: "claude-sonnet-4-6", Temperature: 0.3, MaxTokens: 4096, Purpose: "code"},
		},
		DefaultProvider: "anthropic",
		Tags:            []string{"override"},
		Author:          "tester",
	}
	r.Register(override)

	if r.Packs["budget"].Description != "Overridden budget pack" {
		t.Error("expected pack to be overwritten")
	}
}

func TestList(t *testing.T) {
	r := NewModelPackRegistry()
	packs := r.List()

	if len(packs) != 6 {
		t.Errorf("expected 6 packs, got %d", len(packs))
	}

	// Verify sorted order.
	for i := 1; i < len(packs); i++ {
		if packs[i-1].Name >= packs[i].Name {
			t.Errorf("packs not sorted: %q >= %q", packs[i-1].Name, packs[i].Name)
		}
	}
}

func TestFormatPack(t *testing.T) {
	r := NewModelPackRegistry()
	pack := r.Packs["balanced"]

	output := FormatPack(pack)

	if !strings.Contains(output, `"balanced"`) {
		t.Error("output should contain pack name")
	}
	if !strings.Contains(output, "claude-sonnet-4-6") {
		t.Error("output should contain sonnet model")
	}
	if !strings.Contains(output, "claude-haiku-4-5") {
		t.Error("output should contain haiku model")
	}
	if !strings.Contains(output, "Provider: anthropic") {
		t.Error("output should contain provider")
	}
	if !strings.Contains(output, "code:") {
		t.Error("output should contain 'code:' role")
	}
	if !strings.Contains(output, "temp:") {
		t.Error("output should contain temperature")
	}
}

func TestFormatPack_Nil(t *testing.T) {
	output := FormatPack(nil)
	if output != "" {
		t.Errorf("expected empty string for nil pack, got %q", output)
	}
}

func TestEstimateCost(t *testing.T) {
	r := NewModelPackRegistry()

	costBudget := EstimateCost(r.Packs["budget"], 100000)
	costQuality := EstimateCost(r.Packs["quality"], 100000)
	costLocal := EstimateCost(r.Packs["local"], 100000)

	if costQuality <= costBudget {
		t.Errorf("quality (%f) should cost more than budget (%f)", costQuality, costBudget)
	}
	if costLocal != 0.0 {
		t.Errorf("local pack should be free, got %f", costLocal)
	}
	if costBudget <= 0 {
		t.Errorf("budget cost should be positive, got %f", costBudget)
	}
}

func TestEstimateCost_Nil(t *testing.T) {
	cost := EstimateCost(nil, 100000)
	if cost != 0.0 {
		t.Errorf("expected 0 for nil pack, got %f", cost)
	}
}

func TestEstimateCost_Empty(t *testing.T) {
	pack := &ModelPack{
		Name:   "empty",
		Models: map[string]ModelRole{},
	}
	cost := EstimateCost(pack, 100000)
	if cost != 0.0 {
		t.Errorf("expected 0 for empty pack, got %f", cost)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Use a temp directory for testing.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Ensure .hawk directory exists.
	os.MkdirAll(filepath.Join(tmpDir, ".hawk"), 0o755)

	r := NewModelPackRegistry()

	// Register a custom pack.
	r.Register(&ModelPack{
		Name:        "test-pack",
		Description: "For testing save/load",
		Models: map[string]ModelRole{
			"code": {Provider: "test", Model: "test-model", Temperature: 0.5, MaxTokens: 1000, Purpose: "test"},
		},
		DefaultProvider: "test",
		Tags:            []string{"test"},
		Author:          "tester",
	})
	r.ActivePack = "test-pack"

	// Save.
	if err := r.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists.
	fp := filepath.Join(tmpDir, ".hawk", "model_packs.json")
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("expected file to exist at %s", fp)
	}

	// Load into a fresh registry.
	r2 := NewModelPackRegistry()
	if err := r2.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if r2.ActivePack != "test-pack" {
		t.Errorf("expected active pack 'test-pack' after load, got %q", r2.ActivePack)
	}
	if _, ok := r2.Packs["test-pack"]; !ok {
		t.Error("expected test-pack to be loaded")
	}
	// Built-in packs should still be present.
	if _, ok := r2.Packs["default"]; !ok {
		t.Error("expected built-in 'default' pack to persist after load")
	}
}

func TestLoad_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	r := NewModelPackRegistry()
	// Should not error when no file exists.
	if err := r.Load(); err != nil {
		t.Fatalf("Load() should not error when file missing: %v", err)
	}
}

func TestCompare(t *testing.T) {
	r := NewModelPackRegistry()
	output := r.Compare("budget", "quality")

	if !strings.Contains(output, "budget") {
		t.Error("compare should contain pack A name")
	}
	if !strings.Contains(output, "quality") {
		t.Error("compare should contain pack B name")
	}
	if !strings.Contains(output, "code") {
		t.Error("compare should contain role 'code'")
	}
	if !strings.Contains(output, "Provider") {
		t.Error("compare should contain 'Provider'")
	}
	if !strings.Contains(output, "Est. Cost") {
		t.Error("compare should contain cost estimate")
	}
}

func TestCompare_NotFound(t *testing.T) {
	r := NewModelPackRegistry()

	output := r.Compare("nonexistent", "budget")
	if !strings.Contains(output, "not found") {
		t.Errorf("expected 'not found' in output, got: %s", output)
	}

	output = r.Compare("budget", "nonexistent")
	if !strings.Contains(output, "not found") {
		t.Errorf("expected 'not found' in output, got: %s", output)
	}
}

func TestModelPackConcurrentAccess(t *testing.T) {
	r := NewModelPackRegistry()
	var wg sync.WaitGroup

	// Concurrent reads.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.GetModel("code")
			_ = r.List()
		}()
	}

	// Concurrent writes.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			packs := []string{"default", "budget", "quality", "speed", "local", "balanced"}
			_ = r.SetActive(packs[n%len(packs)])
		}(i)
	}

	// Concurrent register.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Register(&ModelPack{
				Name:   fmt.Sprintf("concurrent-%d", n),
				Models: map[string]ModelRole{"code": {Model: "test"}},
			})
		}(i)
	}

	wg.Wait()
}

func TestAllRolesPresent(t *testing.T) {
	r := NewModelPackRegistry()
	expectedRoles := []string{"code", "chat", "summarize", "review", "plan", "debug"}

	for name, pack := range r.Packs {
		for _, role := range expectedRoles {
			if _, ok := pack.Models[role]; !ok {
				t.Errorf("pack %q missing role %q", name, role)
			}
		}
	}
}

func TestLocalPackUsesOllama(t *testing.T) {
	r := NewModelPackRegistry()
	pack := r.Packs["local"]

	if pack.DefaultProvider != "ollama" {
		t.Errorf("local pack should use ollama provider, got %q", pack.DefaultProvider)
	}
	for role, mr := range pack.Models {
		if mr.Provider != "ollama" {
			t.Errorf("local pack role %q should use ollama provider, got %q", role, mr.Provider)
		}
	}
}

func TestSpeedPackUsesHaiku(t *testing.T) {
	r := NewModelPackRegistry()
	pack := r.Packs["speed"]

	for role, mr := range pack.Models {
		if !strings.Contains(mr.Model, "haiku") {
			t.Errorf("speed pack role %q should use haiku, got %q", role, mr.Model)
		}
	}
}
