package config

import "testing"

func TestToolPresetByName(t *testing.T) {
	minimal, ok := ToolPresetByName("minimal")
	if !ok {
		t.Fatal("expected 'minimal' preset to exist")
	}
	if len(minimal.Tools) == 0 {
		t.Error("minimal preset should have tools")
	}

	readonly, ok := ToolPresetByName("readonly")
	if !ok || len(readonly.Tools) == 0 {
		t.Error("expected 'readonly' preset with tools")
	}

	full, ok := ToolPresetByName("full")
	if !ok {
		t.Fatal("expected 'full' preset to exist")
	}
	if full.Tools != nil {
		t.Error("full preset should have nil tools (all allowed)")
	}

	if _, ok := ToolPresetByName("nonexistent"); ok {
		t.Error("unknown preset should return ok=false")
	}
}

func TestMergeSettings_DeploymentRouting(t *testing.T) {
	on := true
	merged := MergeSettings(Settings{}, Settings{DeploymentRouting: &on})
	if merged.DeploymentRouting == nil || !*merged.DeploymentRouting {
		t.Error("override DeploymentRouting should propagate to base")
	}

	// Nil override should not clobber base.
	base := MergeSettings(Settings{DeploymentRouting: &on}, Settings{})
	if base.DeploymentRouting == nil || !*base.DeploymentRouting {
		t.Error("nil override should preserve base DeploymentRouting")
	}
}
