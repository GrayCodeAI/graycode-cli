package cmd

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/engine"
)

func TestAutonomyTierDescriptions_PlainLanguage(t *testing.T) {
	cases := []struct {
		level engine.AutonomyLevel
		need  []string
	}{
		{engine.AutonomyBasic, []string{"Read and search", "need your approval"}},
		{engine.AutonomySemi, []string{"edit files automatically", "Terminal commands need your approval"}},
		{engine.AutonomyFull, []string{"most terminal commands", "Risky commands still"}},
		{engine.AutonomyYOLO, []string{"Almost no approval", "fully trust"}},
	}
	for _, tc := range cases {
		desc := autonomyTierDescription(tc.level)
		for _, fragment := range tc.need {
			if !strings.Contains(desc, fragment) {
				t.Fatalf("level %v description %q missing %q", tc.level, desc, fragment)
			}
		}
	}
}

func TestFormatAutonomyTierMessage_NoArrowJargon(t *testing.T) {
	msg := formatAutonomyTierMessage(engine.AutonomyFull)
	if strings.Contains(msg, "→") {
		t.Fatalf("expected no arrow jargon, got %q", msg)
	}
	if strings.Contains(msg, "shell auto") {
		t.Fatalf("expected plain language, got %q", msg)
	}
	if !strings.Contains(msg, "Run") {
		t.Fatalf("expected tier name in message, got %q", msg)
	}
}