package sessioncapture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsInstalled(t *testing.T) {
	tm := NewTraceManager(".")
	// Just verify it doesn't panic — result depends on environment
	_ = tm.IsInstalled()
}

func TestIsEnabled_NotEnabled(t *testing.T) {
	dir := t.TempDir()
	tm := NewTraceManager(dir)
	if tm.IsEnabled() {
		t.Error("expected not enabled in empty dir")
	}
}

func TestIsEnabled_Enabled(t *testing.T) {
	dir := t.TempDir()
	// Create .trace/settings.json to simulate enabled state
	traceDir := filepath.Join(dir, ".trace")
	os.MkdirAll(traceDir, 0o755)
	os.WriteFile(filepath.Join(traceDir, "settings.json"), []byte(`{"enabled":true}`), 0o644)

	tm := NewTraceManager(dir)
	if !tm.IsEnabled() {
		t.Error("expected enabled when .trace/settings.json exists")
	}
}

func TestStatus_NotInstalled(t *testing.T) {
	// Use a PATH that won't have trace
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	tm := NewTraceManager(".")
	status := tm.Status()
	if status != "not installed" {
		t.Errorf("expected 'not installed', got %q", status)
	}
}

func TestStatus_InstalledNotEnabled(t *testing.T) {
	tm := NewTraceManager(t.TempDir())
	if !tm.IsInstalled() {
		t.Skip("trace not installed in this environment")
	}
	status := tm.Status()
	if status == "not installed" {
		t.Error("trace is installed but status says not installed")
	}
}

func TestFormatStatus_NotInstalled(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	tm := NewTraceManager(".")
	result := tm.FormatStatus()
	if result != "Session capture: disabled (trace not installed)" {
		t.Errorf("unexpected format: %q", result)
	}
}

func TestFormatStatus_Enabled(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".trace"), 0o755)
	os.WriteFile(filepath.Join(dir, ".trace", "settings.json"), []byte(`{}`), 0o644)

	tm := NewTraceManager(dir)
	if !tm.IsInstalled() {
		t.Skip("trace not installed")
	}
	result := tm.FormatStatus()
	if result != "Session capture: active (trace recording sessions)" {
		t.Errorf("unexpected: %q", result)
	}
}

func TestAutoSetup_NoTrace(t *testing.T) {
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", origPath)

	tm := NewTraceManager(t.TempDir())
	err := tm.AutoSetup()
	if err != nil {
		t.Errorf("AutoSetup should silently skip when trace not installed, got: %v", err)
	}
}

func TestAutoSetup_AlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".trace"), 0o755)
	os.WriteFile(filepath.Join(dir, ".trace", "settings.json"), []byte(`{}`), 0o644)

	tm := NewTraceManager(dir)
	if !tm.IsInstalled() {
		t.Skip("trace not installed")
	}
	// Should be a no-op
	err := tm.AutoSetup()
	if err != nil {
		t.Errorf("AutoSetup should skip when already enabled, got: %v", err)
	}
}
