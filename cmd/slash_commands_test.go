package cmd

import (
	"strings"
	"testing"
)

func TestSlashCommands_NotEmpty(t *testing.T) {
	t.Parallel()
	cmds := slashCommands()
	if len(cmds) == 0 {
		t.Fatal("slashCommands() should not be empty")
	}
	for _, c := range cmds {
		if !strings.HasPrefix(c, "/") {
			t.Errorf("command %q should start with /", c)
		}
	}
}

func TestSlashCommands_ContainsEssentials(t *testing.T) {
	t.Parallel()
	cmds := slashCommands()
	essential := []string{"/help", "/exit", "/clear", "/model", "/version", "/undo"}
	for _, e := range essential {
		found := false
		for _, c := range cmds {
			if c == e {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("slashCommands() missing essential command %q", e)
		}
	}
}

func TestSlashSuggestions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		wantAny bool
	}{
		{"/he", true},
		{"/mo", true},
		{"/ex", true},
		{"/zzz", false},
		{"hello", false},
		{"/", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			suggestions := slashSuggestions(tt.input)
			if tt.wantAny && len(suggestions) == 0 {
				t.Errorf("slashSuggestions(%q) = empty, want results", tt.input)
			}
			if !tt.wantAny && len(suggestions) > 0 {
				t.Errorf("slashSuggestions(%q) = %v, want empty", tt.input, suggestions)
			}
		})
	}
}

func TestHasString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		values []string
		want   string
		result bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b", "c"}, "d", false},
		{nil, "a", false},
		{[]string{}, "a", false},
	}
	for _, tt := range tests {
		got := hasString(tt.values, tt.want)
		if got != tt.result {
			t.Errorf("hasString(%v, %q) = %v, want %v", tt.values, tt.want, got, tt.result)
		}
	}
}


func TestBranchSummary(t *testing.T) {
	// May produce output or empty depending on whether we're in a git repo
	summary := branchSummary()
	_ = summary // just verify no panic
}

func TestFilesSummary(t *testing.T) {
	summary := filesSummary()
	_ = summary // just verify no panic
}

func TestHooksSummary(t *testing.T) {
	t.Parallel()
	summary := hooksSummary()
	_ = summary
}
