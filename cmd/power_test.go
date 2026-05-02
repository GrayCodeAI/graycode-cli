package cmd

import (
	"strings"
	"testing"
)

func TestPowerPresetRange(t *testing.T) {
	// Every level 1-10 should return a valid config
	for level := 1; level <= 10; level++ {
		config := PowerPreset(level)
		if config.Level != level {
			t.Errorf("PowerPreset(%d).Level = %d", level, config.Level)
		}
		if config.Model == "" {
			t.Errorf("PowerPreset(%d).Model is empty", level)
		}
		if config.MaxTokens <= 0 {
			t.Errorf("PowerPreset(%d).MaxTokens should be positive", level)
		}
		if config.ContextWindow <= 0 {
			t.Errorf("PowerPreset(%d).ContextWindow should be positive", level)
		}
		if config.BudgetUSD <= 0 {
			t.Errorf("PowerPreset(%d).BudgetUSD should be positive", level)
		}
	}
}

func TestPowerPresetClamps(t *testing.T) {
	// Below 1 should clamp to 1
	low := PowerPreset(0)
	if low.Level != 1 {
		t.Errorf("PowerPreset(0) should clamp to level 1, got %d", low.Level)
	}

	// Above 10 should clamp to 10
	high := PowerPreset(15)
	if high.Level != 10 {
		t.Errorf("PowerPreset(15) should clamp to level 10, got %d", high.Level)
	}
}

func TestPowerPresetScaling(t *testing.T) {
	// Higher power levels should have >= context, budget, turns
	prev := PowerPreset(1)
	for level := 2; level <= 10; level++ {
		curr := PowerPreset(level)
		if curr.ContextWindow < prev.ContextWindow {
			t.Errorf("level %d context (%d) < level %d context (%d)",
				level, curr.ContextWindow, level-1, prev.ContextWindow)
		}
		if curr.BudgetUSD < prev.BudgetUSD {
			t.Errorf("level %d budget ($%.2f) < level %d budget ($%.2f)",
				level, curr.BudgetUSD, level-1, prev.BudgetUSD)
		}
		if curr.MaxTurns < prev.MaxTurns {
			t.Errorf("level %d maxTurns (%d) < level %d maxTurns (%d)",
				level, curr.MaxTurns, level-1, prev.MaxTurns)
		}
		prev = curr
	}
}

func TestDescribePower(t *testing.T) {
	desc := DescribePower(5)
	if !strings.Contains(desc, "Power 5") {
		t.Errorf("description should mention power level, got %q", desc)
	}
	if !strings.Contains(desc, "sonnet") {
		t.Errorf("level 5 description should mention sonnet model, got %q", desc)
	}
	if !strings.Contains(desc, "$") {
		t.Errorf("description should mention budget, got %q", desc)
	}
	if !strings.Contains(desc, "context") {
		t.Errorf("description should mention context, got %q", desc)
	}
}

func TestDescribePowerHighLevel(t *testing.T) {
	desc := DescribePower(10)
	if !strings.Contains(desc, "Power 10") {
		t.Errorf("description should mention power level, got %q", desc)
	}
	if !strings.Contains(desc, "opus") {
		t.Errorf("level 10 description should mention opus model, got %q", desc)
	}
	if !strings.Contains(desc, "thorough") {
		t.Errorf("level 10 description should mention thorough review, got %q", desc)
	}
}

func TestPowerDefaultIsFive(t *testing.T) {
	config := PowerPreset(5)
	if config.Model == "" {
		t.Error("default power level 5 should have a model set")
	}
	if config.ReviewDepth != "quick" {
		t.Errorf("level 5 review depth should be 'quick', got %q", config.ReviewDepth)
	}
	if config.AutoApply {
		t.Error("level 5 should not auto-apply")
	}
}
