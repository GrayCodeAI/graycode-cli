package hooks

import "testing"

func TestCanonicalEvent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"PreToolUse", string(EventPreTool)},
		{"pre_tool", string(EventPreTool)},
		{"pre_tool_use", string(EventPreTool)},
		{"PostToolUse", string(EventPostTool)},
		{"subagent_start", string(EventSubagentStart)},
		{"SubagentStart", string(EventSubagentStart)},
		{"failure", string(EventFailure)},
	}
	for _, tc := range cases {
		if got := CanonicalEvent(tc.in); got != tc.want {
			t.Errorf("CanonicalEvent(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestEventsMatch(t *testing.T) {
	if !EventsMatch("PreToolUse", "pre_tool") {
		t.Fatal("PreToolUse should match pre_tool")
	}
	if EventsMatch("pre_tool", "post_tool") {
		t.Fatal("should not match")
	}
}

func TestDecisionMatcherVendorAlias(t *testing.T) {
	m := DecisionMatcher{Events: []string{"PreToolUse"}, ToolNames: []string{"Write"}}
	if !m.Match("pre_tool", "Write") {
		t.Fatal("matcher PreToolUse should match runtime pre_tool")
	}
}
