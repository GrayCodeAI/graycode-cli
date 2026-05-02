package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewHealthRouter(t *testing.T) {
	hr := NewHealthRouter()
	if hr == nil {
		t.Fatal("expected non-nil health router")
	}
	if len(hr.tiers) != 3 {
		t.Errorf("expected 3 default tiers, got %d", len(hr.tiers))
	}
	if hr.tiers[0].Name != "light" {
		t.Errorf("expected first tier to be 'light', got %q", hr.tiers[0].Name)
	}
}

func TestHealthRouter_SelectTier(t *testing.T) {
	hr := NewHealthRouter()

	tests := []struct {
		name     string
		health   CodeHealth
		expected string
	}{
		{
			name:     "simple file",
			health:   CodeHealth{Complexity: 5.0, FileSize: 50, Dependencies: 2, Language: "go"},
			expected: "light",
		},
		{
			name:     "moderate file",
			health:   CodeHealth{Complexity: 20.0, FileSize: 300, Dependencies: 10, Language: "go"},
			expected: "standard",
		},
		{
			name:     "complex file",
			health:   CodeHealth{Complexity: 50.0, FileSize: 800, Dependencies: 25, Language: "go"},
			expected: "heavy",
		},
		{
			name:     "boundary light/standard",
			health:   CodeHealth{Complexity: 10.0},
			expected: "light",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier := hr.SelectTier(tt.health)
			if tier != tt.expected {
				t.Errorf("SelectTier() = %q, want %q", tier, tt.expected)
			}
		})
	}
}

func TestHealthRouter_ComputeHealth(t *testing.T) {
	hr := NewHealthRouter()

	dir := t.TempDir()

	// Write a small Go file
	smallFile := filepath.Join(dir, "small.go")
	os.WriteFile(smallFile, []byte(`package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`), 0o644)

	health := hr.ComputeHealth(smallFile)
	if health.Language != "go" {
		t.Errorf("expected language=go, got %q", health.Language)
	}
	if health.FileSize == 0 {
		t.Error("expected non-zero file size")
	}
	if health.Dependencies != 1 {
		t.Errorf("expected 1 dependency (import), got %d", health.Dependencies)
	}

	// Write a larger Go file with multiple imports
	largeFile := filepath.Join(dir, "large.go")
	os.WriteFile(largeFile, []byte(`package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func processA() {
	if true {
		for i := 0; i < 10; i++ {
			if i > 5 {
				fmt.Println(i)
			}
		}
	}
}

func processB(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	}
}
`), 0o644)

	healthLarge := hr.ComputeHealth(largeFile)
	if healthLarge.Dependencies < 5 {
		t.Errorf("expected at least 5 dependencies, got %d", healthLarge.Dependencies)
	}
	if healthLarge.Complexity <= health.Complexity {
		t.Errorf("large file should have higher complexity (%f) than small file (%f)",
			healthLarge.Complexity, health.Complexity)
	}
}

func TestHealthRouter_ModelForTask(t *testing.T) {
	hr := NewHealthRouter()

	dir := t.TempDir()

	// A tiny file should route to light tier
	tinyFile := filepath.Join(dir, "tiny.go")
	os.WriteFile(tinyFile, []byte("package main\n\nfunc main() {}\n"), 0o644)

	model := hr.ModelForTask(tinyFile, "claude-sonnet-4-20250514")
	// Should select a light-tier model since complexity is low
	lightModels := map[string]bool{
		"claude-3-5-haiku-20241022": true,
		"gpt-4o-mini":              true,
		"gemini-2.5-flash":         true,
	}
	if !lightModels[model] {
		t.Errorf("expected a light-tier model for tiny file, got %q", model)
	}

	// If primaryModel is in the selected tier, it should be returned
	model2 := hr.ModelForTask(tinyFile, "gpt-4o-mini")
	if model2 != "gpt-4o-mini" {
		t.Errorf("expected primary model 'gpt-4o-mini' since it's in light tier, got %q", model2)
	}
}
