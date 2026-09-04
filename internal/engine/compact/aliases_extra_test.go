package compact

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/types"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	// Should match DefaultCompactConfig
	expected := DefaultCompactConfig()
	if cfg.ContextWindowSize != expected.ContextWindowSize {
		t.Errorf("ContextWindowSize mismatch")
	}
}

func TestDefaultMicroConfig(t *testing.T) {
	cfg := DefaultMicroConfig()
	expected := DefaultMicroCompactConfig()
	if cfg.TimeGapMins != expected.TimeGapMins {
		t.Errorf("TimeGapMins mismatch")
	}
	if cfg.KeepRecent != expected.KeepRecent {
		t.Errorf("KeepRecent mismatch")
	}
}

func TestDefaultAPIConfig(t *testing.T) {
	cfg := DefaultAPIConfig()
	expected := DefaultAPICompactConfig()
	if cfg.TriggerTokens != expected.TriggerTokens {
		t.Errorf("TriggerTokens mismatch")
	}
	if cfg.TriggerTokens <= 0 {
		t.Error("expected positive TriggerTokens")
	}
}

func TestBuildPrompt(t *testing.T) {
	// Test with each variant
	variants := []Variant{CompactBase, CompactPartial, CompactUpTo}
	for _, v := range variants {
		result := BuildPrompt(v)
		expected := BuildCompactPrompt(v)
		if result != expected {
			t.Errorf("BuildPrompt(%v) != BuildCompactPrompt(%v)", v, v)
		}
	}
}

func TestFormatSummary(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"  trimmed  "},
		{"no change"},
		{""},
	}
	for _, tt := range tests {
		result := FormatSummary(tt.input)
		expected := FormatCompactSummary(tt.input)
		if result != expected {
			t.Errorf("FormatSummary(%q) != FormatCompactSummary(%q)", tt.input, tt.input)
		}
	}
}

func TestDefaultAPICompactConfig(t *testing.T) {
	cfg := DefaultAPICompactConfig()
	if cfg.TriggerTokens <= 0 {
		t.Error("expected positive TriggerTokens")
	}
}

func TestCountClearableToolResults(t *testing.T) {
	// Empty messages
	count := CountClearableToolResults(nil)
	if count != 0 {
		t.Errorf("CountClearableToolResults(nil) = %d, want 0", count)
	}
}

func TestIsThinkingMessage(t *testing.T) {
	// Empty message
	result := isThinkingMessage(types.GraycodeRouterMessage{})
	if result {
		t.Error("expected false for empty message")
	}
}

func TestDefaultMicroCompactConfig(t *testing.T) {
	cfg := DefaultMicroCompactConfig()
	if cfg.TimeGapMins <= 0 {
		t.Error("expected positive TimeGapMins")
	}
	if cfg.KeepRecent <= 0 {
		t.Error("expected positive KeepRecent")
	}
}

func TestHasTimeGap(t *testing.T) {
	// Empty messages should have no time gap
	result := HasTimeGap(nil, 0)
	if result {
		t.Error("expected false for empty messages")
	}
}

func TestDefaultSessionMemoryConfig(t *testing.T) {
	cfg := DefaultSessionMemoryConfig()
	if cfg.MaxTokens <= 0 {
		t.Error("expected positive MaxTokens")
	}
}

func TestSessionMemoryPath(t *testing.T) {
	path := SessionMemoryPath("test-session")
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestReadSessionMemory_NonExistent(t *testing.T) {
	_, err := ReadSessionMemory("nonexistent-session-12345")
	if err == nil {
		t.Error("expected error for non-existent session memory")
	}
}

func TestIsCompactBoundary(t *testing.T) {
	// Empty message
	result := IsCompactBoundary(types.GraycodeRouterMessage{})
	if result {
		t.Error("expected false for empty message")
	}
}

func TestFilterCompactBoundaries_Empty(t *testing.T) {
	// Empty messages
	result := FilterCompactBoundaries(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}
