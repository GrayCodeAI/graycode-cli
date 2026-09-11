package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func TestAdaptivePrompt_New(t *testing.T) {
	testutil.IsolateStorage(t)

	ap := NewAdaptivePrompt()
	if ap == nil {
		t.Fatal("NewAdaptivePrompt returned nil")
	}
	if ap.path == "" {
		t.Error("path should not be empty")
	}
}

func TestAdaptivePrompt_LearnFromFeedback(t *testing.T) {
	testutil.IsolateStorage(t)

	ap := NewAdaptivePrompt()

	tests := []struct {
		name     string
		input    string
		wantRule bool
		polarity string
	}{
		{"dont pattern", "don't add comments to the code", true, "dont"},
		{"never pattern", "never use fmt.Println for logging", true, "dont"},
		{"always pattern", "always use structured errors", true, "do"},
		{"please always pattern", "please always use table-driven tests in this project", true, "do"},
		{"too short", "don't x", false, ""},
		{"no directive", "can you help me fix this bug?", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := ap.Count()
			ap.LearnFromFeedback(tt.input)
			after := ap.Count()

			if tt.wantRule && after <= before {
				t.Errorf("expected rule to be learned from %q", tt.input)
			}
			if !tt.wantRule && after > before {
				t.Errorf("did not expect rule from %q", tt.input)
			}
		})
	}
}

func TestAdaptivePrompt_FormatForPrompt(t *testing.T) {
	testutil.IsolateStorage(t)

	ap := NewAdaptivePrompt()
	ap.LearnFromFeedback("don't add trailing whitespace to files")
	ap.LearnFromFeedback("always use context.Context as first parameter")

	result := ap.FormatForPrompt()
	if result == "" {
		t.Error("FormatForPrompt should produce output after learning")
	}
	if !strings.Contains(strings.ToLower(result), "whitespace") {
		t.Error("should contain learned rule about whitespace")
	}
}

func TestAdaptivePrompt_Count(t *testing.T) {
	testutil.IsolateStorage(t)

	ap := NewAdaptivePrompt()
	if ap.Count() != 0 {
		t.Errorf("Count() = %d, want 0 for new prompt", ap.Count())
	}

	ap.LearnFromFeedback("don't use global variables in library code")
	if ap.Count() < 1 {
		t.Error("Count should increase after learning")
	}
}

func TestAdaptivePrompt_Persistence(t *testing.T) {
	testutil.IsolateStorage(t)

	ap1 := NewAdaptivePrompt()
	ap1.LearnFromFeedback("don't add unnecessary dependencies")

	// Create a new instance — should load from disk
	ap2 := NewAdaptivePrompt()
	if ap2.Count() != ap1.Count() {
		t.Errorf("persisted count = %d, want %d", ap2.Count(), ap1.Count())
	}
}
