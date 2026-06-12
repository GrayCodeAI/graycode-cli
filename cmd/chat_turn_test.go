package cmd

import "testing"

func TestTurnHadThinkingOnly(t *testing.T) {
	cases := []struct {
		name string
		msgs []displayMsg
		want bool
	}{
		{
			name: "thinking without answer",
			msgs: []displayMsg{
				{role: "user", content: "hi"},
				{role: "thinking", content: "plan"},
			},
			want: true,
		},
		{
			name: "thinking with assistant",
			msgs: []displayMsg{
				{role: "user", content: "hi"},
				{role: "thinking", content: "plan"},
				{role: "assistant", content: "hello"},
			},
			want: false,
		},
		{
			name: "tool turn",
			msgs: []displayMsg{
				{role: "user", content: "hi"},
				{role: "thinking", content: "plan"},
				{role: "tool_use", content: "Read"},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := turnHadThinkingOnly(tc.msgs); got != tc.want {
				t.Fatalf("turnHadThinkingOnly() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStripCurrentTurnThinking(t *testing.T) {
	msgs := []displayMsg{
		{role: "user", content: "old"},
		{role: "assistant", content: "prior"},
		{role: "user", content: "hi"},
		{role: "thinking", content: "plan"},
	}
	got := stripCurrentTurnThinking(msgs)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[2].role != "user" || got[2].content != "hi" {
		t.Fatalf("last msg = %+v, want user hi", got[2])
	}
}
