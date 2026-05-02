package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRunCommandGo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example"), 0o644)

	got := DetectRunCommand(dir)
	if got != "go test ./..." {
		t.Errorf("expected 'go test ./...' for go.mod project, got %q", got)
	}
}

func TestDetectRunCommandNode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)

	got := DetectRunCommand(dir)
	if got != "npm test" {
		t.Errorf("expected 'npm test' for package.json project, got %q", got)
	}
}

func TestDetectRunCommandEmpty(t *testing.T) {
	dir := t.TempDir()

	got := DetectRunCommand(dir)
	if got != "" {
		t.Errorf("expected empty string for unknown project, got %q", got)
	}
}

func TestDefaultVibeConfig(t *testing.T) {
	cfg := DefaultVibeConfig()
	if !cfg.Enabled {
		t.Error("default vibe config should be enabled")
	}
	if !cfg.AutoApply {
		t.Error("default vibe config should have AutoApply true")
	}
	if !cfg.AutoRun {
		t.Error("default vibe config should have AutoRun true")
	}
	if cfg.ShowDiffs {
		t.Error("default vibe config should have ShowDiffs false")
	}
	if cfg.MaxIterations != 10 {
		t.Errorf("default vibe config MaxIterations should be 10, got %d", cfg.MaxIterations)
	}
}

func TestDetectRunCommandPriority(t *testing.T) {
	// When both go.mod and Makefile exist, go.mod should win (checked first)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example"), 0o644)
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("test:"), 0o644)

	got := DetectRunCommand(dir)
	if got != "go test ./..." {
		t.Errorf("expected go.mod to take priority, got %q", got)
	}
}
