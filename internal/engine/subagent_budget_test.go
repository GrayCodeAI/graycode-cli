package engine

import (
	"testing"
)

func TestSubAgentDefaultConfig(t *testing.T) {
	cfg := DefaultSubAgentConfig()
	if cfg.ExploreMaxTurns != 15 {
		t.Errorf("ExploreMaxTurns = %d, want 15", cfg.ExploreMaxTurns)
	}
	if cfg.GeneralMaxTurns != 20 {
		t.Errorf("GeneralMaxTurns = %d, want 20", cfg.GeneralMaxTurns)
	}
	if cfg.MaxDepth != 1 {
		t.Errorf("MaxDepth = %d, want 1", cfg.MaxDepth)
	}
}

func TestSubAgentBudgetExceeded(t *testing.T) {
	cfg := DefaultSubAgentConfig()
	b := NewSubAgentBudget(SubAgentExplore, cfg)

	if b.ShouldSynthesize() {
		t.Fatal("ShouldSynthesize() = true at turn 0, want false")
	}
	if b.Remaining() != 15 {
		t.Errorf("Remaining() = %d, want 15", b.Remaining())
	}

	// Use up all turns.
	for i := 0; i < 15; i++ {
		b.Tick()
	}

	if !b.ShouldSynthesize() {
		t.Fatal("ShouldSynthesize() = false at turn 15, want true")
	}
	if b.Remaining() != 0 {
		t.Errorf("Remaining() = %d, want 0", b.Remaining())
	}

	// Over budget — still should synthesize.
	b.Tick()
	if !b.ShouldSynthesize() {
		t.Fatal("ShouldSynthesize() = false at turn 16, want true")
	}
	if b.Remaining() != 0 {
		t.Errorf("Remaining() = %d after exceeding budget, want 0", b.Remaining())
	}
}

func TestSubAgentFilterToolsExplore(t *testing.T) {
	available := []string{"Read", "Grep", "Glob", "LS", "Bash", "Write", "Edit", "Agent", "MultiAgent"}
	filtered := FilterToolsForMode(SubAgentExplore, available)

	expected := map[string]bool{
		"Read": true,
		"Grep": true,
		"Glob": true,
		"LS":   true,
		"Bash": true,
	}

	if len(filtered) != len(expected) {
		t.Fatalf("FilterToolsForMode(explore) returned %d tools, want %d: %v", len(filtered), len(expected), filtered)
	}
	for _, tool := range filtered {
		if !expected[tool] {
			t.Errorf("FilterToolsForMode(explore) included unexpected tool %q", tool)
		}
	}

	// Write/Edit/Agent should NOT be present.
	for _, tool := range filtered {
		if tool == "Write" || tool == "Edit" || tool == "Agent" {
			t.Errorf("FilterToolsForMode(explore) should not include %q", tool)
		}
	}
}

func TestSubAgentFilterToolsGeneral(t *testing.T) {
	available := []string{"Read", "Grep", "Glob", "LS", "Bash", "Write", "Edit", "Agent", "MultiAgent"}
	filtered := FilterToolsForMode(SubAgentGeneral, available)

	// General mode allows all tools in the allowlist.
	expected := map[string]bool{
		"Read":       true,
		"Grep":       true,
		"Glob":       true,
		"LS":         true,
		"Bash":       true,
		"Write":      true,
		"Edit":       true,
		"Agent":      true,
		"MultiAgent": true,
	}

	if len(filtered) != len(expected) {
		t.Fatalf("FilterToolsForMode(general) returned %d tools, want %d: %v", len(filtered), len(expected), filtered)
	}
	for _, tool := range filtered {
		if !expected[tool] {
			t.Errorf("FilterToolsForMode(general) included unexpected tool %q", tool)
		}
	}
}
