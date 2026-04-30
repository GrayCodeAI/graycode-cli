package prompt

import (
	"strings"
	"testing"
)

func TestSystemPromptContainsEssentials(t *testing.T) {
	s := System()
	for _, want := range []string{"hawk", "Bash", "Read", "Write", "Edit", "Glob", "Grep", "Safety"} {
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
