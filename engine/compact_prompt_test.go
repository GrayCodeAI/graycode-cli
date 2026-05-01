package engine

import (
	"strings"
	"testing"
)

func TestBuildCompactPrompt_Base(t *testing.T) {
	prompt := BuildCompactPrompt(CompactBase)
	if !strings.Contains(prompt, "CRITICAL: Respond with TEXT ONLY") {
		t.Error("should contain no-tools preamble")
	}
	if !strings.Contains(prompt, "Chronologically analyze each message") {
		t.Error("should contain base analysis instruction")
	}
	if !strings.Contains(prompt, "Primary Request & Intent") {
		t.Error("should contain summary template")
	}
}

func TestBuildCompactPrompt_Partial(t *testing.T) {
	prompt := BuildCompactPrompt(CompactPartial)
	if !strings.Contains(prompt, "Analyze the recent messages") {
		t.Error("should contain partial analysis instruction")
	}
}

func TestFormatCompactSummary_WithTags(t *testing.T) {
	raw := `<analysis>
This is my internal analysis that should be stripped.
I'm thinking through the conversation...
</analysis>

<summary>
The user asked to implement a login feature.
Files modified: auth.go, handler.go.
Next step: add tests.
</summary>`

	result := FormatCompactSummary(raw)
	if strings.Contains(result, "internal analysis") {
		t.Error("analysis block should be stripped")
	}
	if !strings.Contains(result, "login feature") {
		t.Error("summary content should be preserved")
	}
	if !strings.Contains(result, "add tests") {
		t.Error("next step should be preserved")
	}
}

func TestFormatCompactSummary_NoTags(t *testing.T) {
	raw := "Just a plain summary without any tags."
	result := FormatCompactSummary(raw)
	if result != raw {
		t.Errorf("should return as-is when no tags, got %q", result)
	}
}

func TestFormatCompactSummary_OnlyAnalysis(t *testing.T) {
	raw := `<analysis>thinking...</analysis>
The actual summary content here.`

	result := FormatCompactSummary(raw)
	if strings.Contains(result, "thinking") {
		t.Error("analysis should be stripped")
	}
	if !strings.Contains(result, "actual summary") {
		t.Error("remaining content should be kept")
	}
}
