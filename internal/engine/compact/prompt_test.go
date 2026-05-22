package compact

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
	if !strings.Contains(prompt, "## Goal") {
		t.Error("should contain structured summary template")
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
</analysis>
<summary>
## Goal
- test task
</summary>`
	result := FormatCompactSummary(raw)
	if strings.Contains(result, "analysis") {
		t.Error("should strip <analysis> block")
	}
	if !strings.Contains(result, "## Goal") {
		t.Error("should keep <summary> content")
	}
	if !strings.Contains(result, "test task") {
		t.Error("should keep summary text")
	}
}

func TestFormatCompactSummary_NoTags(t *testing.T) {
	raw := "plain text response"
	result := FormatCompactSummary(raw)
	if result != raw {
		t.Errorf("expected %q, got %q", raw, result)
	}
}

func TestFormatCompactSummary_SummaryOnly(t *testing.T) {
	raw := "<summary>just this</summary>"
	result := FormatCompactSummary(raw)
	if result != "just this" {
		t.Errorf("expected 'just this', got %q", result)
	}
}

func TestBuildCompactPrompt_UpTo(t *testing.T) {
	prompt := BuildCompactPrompt(CompactUpTo)
	if !strings.Contains(prompt, "Chronologically") {
		t.Error("UpTo variant should use base analysis (default)")
	}
}
