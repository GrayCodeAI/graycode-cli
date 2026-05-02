package prompt

import (
	"strings"
	"testing"
)

func TestSystemPromptContainsEssentials(t *testing.T) {
	s := System()
	// Tool details are now in prompts/templates/; System() retains only
	// identity, environment, system instructions, and safety.
	for _, want := range []string{"hawk", "Environment", "System", "Safety"} {
		if !strings.Contains(s, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
}

func TestSystemPromptContainsDate(t *testing.T) {
	s := System()
	if !strings.Contains(s, "Date:") {
		t.Error("system prompt missing date")
	}
}
