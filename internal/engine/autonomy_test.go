package engine

import "testing"

func TestPresetConfigSupervised(t *testing.T) {
	cfg := PresetConfig(AutonomySupervised)
	if cfg.AutoContinue || cfg.AutoApplyEdits || cfg.AutoExecuteBash || cfg.AutoCommit {
		t.Error("supervised should have all flags false")
	}
}

func TestPresetConfigBasic(t *testing.T) {
	cfg := PresetConfig(AutonomyBasic)
	if !cfg.AutoContinue {
		t.Error("basic should have AutoContinue true")
	}
	if cfg.AutoApplyEdits || cfg.AutoExecuteBash || cfg.AutoCommit {
		t.Error("basic should not auto-apply edits, bash, or commit")
	}
}

func TestPresetConfigSemi(t *testing.T) {
	cfg := PresetConfig(AutonomySemi)
	if !cfg.AutoContinue || !cfg.AutoApplyEdits {
		t.Error("semi should have AutoContinue and AutoApplyEdits true")
	}
	if cfg.AutoExecuteBash || cfg.AutoCommit {
		t.Error("semi should not auto-execute bash or auto-commit")
	}
}

func TestPresetConfigFull(t *testing.T) {
	cfg := PresetConfig(AutonomyFull)
	if !cfg.AutoContinue || !cfg.AutoApplyEdits || !cfg.AutoExecuteBash || !cfg.AutoCommit {
		t.Error("full should have all flags true")
	}
}

func TestPresetConfigYOLO(t *testing.T) {
	cfg := PresetConfig(AutonomyYOLO)
	if !cfg.AutoContinue || !cfg.AutoApplyEdits || !cfg.AutoExecuteBash || !cfg.AutoCommit {
		t.Error("yolo should have all flags true")
	}
}

func TestNeedsPermissionSupervised(t *testing.T) {
	cfg := PresetConfig(AutonomySupervised)
	for _, tool := range []string{"Read", "Write", "Edit", "Bash", "Grep"} {
		if !cfg.NeedsPermission(tool, false) {
			t.Errorf("supervised: %s should need permission", tool)
		}
	}
}

func TestNeedsPermissionBasic(t *testing.T) {
	cfg := PresetConfig(AutonomyBasic)
	// Read-only tools should not need permission.
	for _, tool := range []string{"Read", "Grep", "Glob", "LS", "WebSearch"} {
		if cfg.NeedsPermission(tool, false) {
			t.Errorf("basic: read-only tool %s should NOT need permission", tool)
		}
	}
	// Write tools and Bash should need permission.
	for _, tool := range []string{"Write", "Edit", "Bash"} {
		if !cfg.NeedsPermission(tool, false) {
			t.Errorf("basic: %s should need permission", tool)
		}
	}
}

func TestNeedsPermissionSemi(t *testing.T) {
	cfg := PresetConfig(AutonomySemi)
	// Read and write tools should not need permission.
	for _, tool := range []string{"Read", "Grep", "Write", "Edit"} {
		if cfg.NeedsPermission(tool, false) {
			t.Errorf("semi: %s should NOT need permission", tool)
		}
	}
	// Bash should need permission.
	if !cfg.NeedsPermission("Bash", false) {
		t.Error("semi: Bash should need permission")
	}
}

func TestNeedsPermissionFull(t *testing.T) {
	cfg := PresetConfig(AutonomyFull)
	// Safe bash should not need permission.
	if cfg.NeedsPermission("Bash", true) {
		t.Error("full: safe Bash should NOT need permission")
	}
	// Unsafe (destructive) bash should need permission.
	if !cfg.NeedsPermission("Bash", false) {
		t.Error("full: unsafe Bash should need permission")
	}
	// Non-bash tools should not need permission.
	for _, tool := range []string{"Read", "Write", "Edit", "Grep"} {
		if cfg.NeedsPermission(tool, false) {
			t.Errorf("full: %s should NOT need permission", tool)
		}
	}
}

func TestNeedsPermissionYOLO(t *testing.T) {
	cfg := PresetConfig(AutonomyYOLO)
	for _, tool := range []string{"Read", "Write", "Edit", "Bash", "Grep"} {
		if cfg.NeedsPermission(tool, false) {
			t.Errorf("yolo: %s should NOT need permission", tool)
		}
	}
}

func TestParseAutonomyLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected AutonomyLevel
	}{
		{"0", AutonomySupervised},
		{"supervised", AutonomySupervised},
		{"1", AutonomyBasic},
		{"basic", AutonomyBasic},
		{"2", AutonomySemi},
		{"semi", AutonomySemi},
		{"3", AutonomyFull},
		{"full", AutonomyFull},
		{"4", AutonomyYOLO},
		{"yolo", AutonomyYOLO},
		{"YOLO", AutonomyYOLO},
		{"  Full  ", AutonomyFull},
		{"unknown", AutonomySupervised},
		{"", AutonomySupervised},
	}
	for _, tt := range tests {
		got := ParseAutonomyLevel(tt.input)
		if got != tt.expected {
			t.Errorf("ParseAutonomyLevel(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestAutonomyLevelString(t *testing.T) {
	tests := []struct {
		level    AutonomyLevel
		expected string
	}{
		{AutonomySupervised, "supervised"},
		{AutonomyBasic, "basic"},
		{AutonomySemi, "semi"},
		{AutonomyFull, "full"},
		{AutonomyYOLO, "yolo"},
		{AutonomyLevel(99), "supervised"},
	}
	for _, tt := range tests {
		got := tt.level.String()
		if got != tt.expected {
			t.Errorf("AutonomyLevel(%d).String() = %q, want %q", tt.level, got, tt.expected)
		}
	}
}
