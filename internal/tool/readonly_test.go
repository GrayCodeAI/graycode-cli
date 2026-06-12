package tool

import "testing"

func TestIsReadOnly(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		// Canonical names.
		{"Read", "Read", true},
		{"Grep", "Grep", true},
		{"Glob", "Glob", true},
		{"LS", "LS", true},
		{"WebSearch", "WebSearch", true},
		{"WebFetch", "WebFetch", true},
		{"ToolSearch", "ToolSearch", true},
		// Lowercase / aliased forms.
		{"lowercase read", "read", true},
		{"file_read alias", "file_read", true},
		{"uppercase LS", "ls", true},
		// Not in the allowlist.
		{"Write", "Write", false},
		{"Edit", "Edit", false},
		{"Bash", "Bash", false},
		{"Bash lowercase", "bash", false},
		{"Agent", "Agent", false},
		{"Unknown", "TotallyMadeUpTool", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsReadOnly(c.in); got != c.want {
				t.Fatalf("IsReadOnly(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestReadOnlyToolsSetContainsExpectedNames(t *testing.T) {
	want := []string{"Read", "Grep", "Glob", "LS", "WebSearch", "WebFetch", "ToolSearch"}
	for _, name := range want {
		if !ReadOnlyTools[name] {
			t.Errorf("ReadOnlyTools is missing %q (would cause a regression in classifyToolCalls)", name)
		}
	}
}
