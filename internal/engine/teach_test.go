package engine

import (
	"strings"
	"testing"
)

func TestTeachPromptAugment_Depths(t *testing.T) {
	tests := []struct {
		depth    int
		contains string
	}{
		{1, "one sentence"},
		{2, "what you're doing and why"},
		{3, "reasoning process"},
	}
	for _, tt := range tests {
		result := TeachPromptAugment(tt.depth)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("TeachPromptAugment(%d) = %q, expected to contain %q", tt.depth, result, tt.contains)
		}
	}
}

func TestFormatTeachingMoment(t *testing.T) {
	result := FormatTeachingMoment("do X", "because Y")
	if !strings.Contains(result, "because Y") {
		t.Error("expected reasoning in output")
	}
	if !strings.Contains(result, "do X") {
		t.Error("expected action in output")
	}
	// Verify reasoning comes before action
	reasonIdx := strings.Index(result, "because Y")
	actionIdx := strings.Index(result, "do X")
	if reasonIdx >= actionIdx {
		t.Error("expected reasoning before action")
	}
}

func TestFormatTeachingMoment_EmptyReasoning(t *testing.T) {
	result := FormatTeachingMoment("do X", "")
	if result != "do X" {
		t.Errorf("expected plain action, got %q", result)
	}
}
