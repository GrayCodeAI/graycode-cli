package shellmode

import "testing"

func TestClassifyInput(t *testing.T) {
	tests := []struct {
		input string
		want  Classification
	}{
		// Empty
		{"", ClassNeutral},
		{"   ", ClassNeutral},
		// Agent words
		{"why", ClassAgent},
		{"explain", ClassAgent},
		{"thanks", ClassAgent},
		{"hello", ClassAgent},
		{"refactor", ClassAgent},
		// Shell reserved words → agent
		{"do we have a way to install?", ClassAgent},
		{"in the codebase where is auth?", ClassAgent},
		// Valid commands → shell
		{"ls", ClassShell},
		{"git status", ClassShell},
		{"echo hello world", ClassShell},
		// Multi-word, first not a command → agent
		{"fix the bug in auth", ClassAgent},
		{"what files are here", ClassAgent},
		// Single unknown word → agent (conversational)
		{"asdfgh", ClassAgent},
	}

	for _, tt := range tests {
		got := ClassifyInput(tt.input)
		if got != tt.want {
			t.Errorf("ClassifyInput(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
