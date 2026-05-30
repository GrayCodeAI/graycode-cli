package config

import "testing"

func TestIsMinimalMode(t *testing.T) {
	if IsMinimalMode(Settings{}) {
		t.Error("nil MinimalMode should be false")
	}
	on := true
	if !IsMinimalMode(Settings{MinimalMode: &on}) {
		t.Error("MinimalMode=true should be true")
	}
	off := false
	if IsMinimalMode(Settings{MinimalMode: &off}) {
		t.Error("MinimalMode=false should be false")
	}
}

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

func TestMergeSettings_MinimalMode(t *testing.T) {
	on := true
	merged := MergeSettings(Settings{}, Settings{MinimalMode: &on})
	if !IsMinimalMode(merged) {
		t.Error("override MinimalMode should propagate to base")
	}

	// Nil override should not clobber base.
	base := MergeSettings(Settings{MinimalMode: &on}, Settings{})
	if !IsMinimalMode(base) {
		t.Error("nil override should preserve base MinimalMode")
	}
}

func TestSettingValue_MinimalMode(t *testing.T) {
	on := true
	v, ok := SettingValue(Settings{MinimalMode: &on}, "minimal_mode")
	if !ok || v != "true" {
		t.Errorf("expected 'true', got %q (ok=%v)", v, ok)
	}
	v, ok = SettingValue(Settings{}, "minimalmode")
	if !ok || v != "false" {
		t.Errorf("expected 'false', got %q (ok=%v)", v, ok)
	}
}
