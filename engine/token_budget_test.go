package engine

import (
	"strings"
	"testing"

	"github.com/GrayCodeAI/eyrie/client"
)

func TestDynamicMaxTokens_CodeGenTask(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "Implement a REST API server"},
	}
	got := DynamicMaxTokens(msgs, 200000, "code")
	if got != 16384 {
		t.Errorf("code task: expected 16384, got %d", got)
	}
}

func TestDynamicMaxTokens_TextQuestion(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "What is the difference between a mutex and a semaphore?"},
	}
	got := DynamicMaxTokens(msgs, 200000, "code")
	// Should be reduced to 8192 because the last user message is a question.
	if got != 8192 {
		t.Errorf("text question: expected 8192, got %d", got)
	}
}

func TestDynamicMaxTokens_ToolHeavyPattern(t *testing.T) {
	// Simulate 3 consecutive assistant turns that all used tools.
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "read the files"},
		{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t1", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: "file1 content"}},
		{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t2", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t2", Content: "file2 content"}},
		{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t3", Name: "Grep"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t3", Content: "grep results"}},
		{Role: "user", Content: "now implement the feature"},
	}
	got := DynamicMaxTokens(msgs, 200000, "code")
	if got != 4096 {
		t.Errorf("tool-heavy: expected 4096, got %d", got)
	}
}

func TestDynamicMaxTokens_NotEnoughToolTurns(t *testing.T) {
	// Only 2 tool turns (below the lookback threshold of 3).
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "do something"},
		{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t1", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: "output"}},
		{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t2", Name: "Bash"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t2", Content: "output"}},
		{Role: "user", Content: "implement the feature"},
	}
	got := DynamicMaxTokens(msgs, 200000, "code")
	// With only 2 assistant tool turns, should not trigger tool-heavy reduction.
	if got != 16384 {
		t.Errorf("not-enough-tool-turns: expected 16384, got %d", got)
	}
}

func TestDynamicMaxTokens_MixedToolAndText(t *testing.T) {
	// 3 assistant turns, but one is text-only -- not tool-heavy.
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "help me"},
		{Role: "assistant", Content: "Sure, I can help."},
		{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t1", Name: "Read"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: "output"}},
		{Role: "assistant", Content: "", ToolUse: []client.ToolCall{{ID: "t2", Name: "Bash"}}},
		{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t2", Content: "output"}},
		{Role: "user", Content: "implement it"},
	}
	got := DynamicMaxTokens(msgs, 200000, "code")
	// 3 assistant turns but only 2 have tools -> not tool-heavy.
	if got != 16384 {
		t.Errorf("mixed: expected 16384, got %d", got)
	}
}

func TestDynamicMaxTokens_ContextBudgetLimit(t *testing.T) {
	// Build messages that consume most of the context.
	msgs := []client.EyrieMessage{
		{Role: "user", Content: strings.Repeat("word ", 5000)},
	}
	// Small context window of 8000 tokens.
	got := DynamicMaxTokens(msgs, 8000, "code")
	// Input is ~5000 tokens, remaining is ~3000. Should cap at remaining.
	if got > 8000 {
		t.Errorf("context-limit: should not exceed context size, got %d", got)
	}
	if got >= 16384 {
		t.Errorf("context-limit: should be capped below base 16384, got %d", got)
	}
}

func TestDynamicMaxTokens_VerySmallContext(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: strings.Repeat("word ", 2000)},
	}
	got := DynamicMaxTokens(msgs, 2100, "code")
	// Input tokens are ~2000, remaining is ~100. Floor should kick in at 1024.
	if got < 1024 {
		t.Errorf("small-context: floor should be 1024, got %d", got)
	}
}

func TestDynamicMaxTokens_ZeroContextSize(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "generate code"},
	}
	// contextSize=0 means unknown/unlimited -- should use base budget.
	got := DynamicMaxTokens(msgs, 0, "code")
	if got != 16384 {
		t.Errorf("zero-context: expected 16384, got %d", got)
	}
}

func TestDynamicMaxTokens_ExplainTask(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "generate something"},
	}
	got := DynamicMaxTokens(msgs, 200000, "explain")
	if got != 8192 {
		t.Errorf("explain task: expected 8192, got %d", got)
	}
}

func TestDynamicMaxTokens_ToolTask(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "do it"},
	}
	got := DynamicMaxTokens(msgs, 200000, "tool")
	if got != 4096 {
		t.Errorf("tool task: expected 4096, got %d", got)
	}
}

func TestDynamicMaxTokens_DefaultTask(t *testing.T) {
	msgs := []client.EyrieMessage{
		{Role: "user", Content: "do something"},
	}
	got := DynamicMaxTokens(msgs, 200000, "")
	if got != 16384 {
		t.Errorf("default task: expected 16384, got %d", got)
	}
}

func TestIsRecentToolHeavy(t *testing.T) {
	tests := []struct {
		name string
		msgs []client.EyrieMessage
		want bool
	}{
		{
			name: "empty messages",
			msgs: nil,
			want: false,
		},
		{
			name: "only user messages",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "hello"},
				{Role: "user", Content: "world"},
			},
			want: false,
		},
		{
			name: "three tool turns",
			msgs: []client.EyrieMessage{
				{Role: "assistant", ToolUse: []client.ToolCall{{ID: "1", Name: "Read"}}},
				{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "1"}},
				{Role: "assistant", ToolUse: []client.ToolCall{{ID: "2", Name: "Edit"}}},
				{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "2"}},
				{Role: "assistant", ToolUse: []client.ToolCall{{ID: "3", Name: "Bash"}}},
				{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "3"}},
			},
			want: true,
		},
		{
			name: "two tool one text",
			msgs: []client.EyrieMessage{
				{Role: "assistant", Content: "let me think"},
				{Role: "assistant", ToolUse: []client.ToolCall{{ID: "1", Name: "Read"}}},
				{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "1"}},
				{Role: "assistant", ToolUse: []client.ToolCall{{ID: "2", Name: "Edit"}}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRecentToolHeavy(tt.msgs)
			if got != tt.want {
				t.Errorf("isRecentToolHeavy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTextQuestion(t *testing.T) {
	tests := []struct {
		name string
		msgs []client.EyrieMessage
		want bool
	}{
		{
			name: "empty",
			msgs: nil,
			want: false,
		},
		{
			name: "question mark",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "Is this working?"},
			},
			want: true,
		},
		{
			name: "what prefix",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "What does this function do"},
			},
			want: true,
		},
		{
			name: "explain prefix",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "Explain the architecture"},
			},
			want: true,
		},
		{
			name: "imperative command",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "Implement the REST API"},
			},
			want: false,
		},
		{
			name: "skips tool results",
			msgs: []client.EyrieMessage{
				{Role: "user", Content: "Why does this fail?"},
				{Role: "user", ToolResult: &client.ToolResult{ToolUseID: "t1", Content: "error output"}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTextQuestion(tt.msgs)
			if got != tt.want {
				t.Errorf("isTextQuestion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectBaseTokens(t *testing.T) {
	tests := []struct {
		taskType string
		want     int
	}{
		{"code", 16384},
		{"codegen", 16384},
		{"CODE", 16384},
		{"implement", 16384},
		{"question", 8192},
		{"explain", 8192},
		{"text", 8192},
		{"tool", 4096},
		{"tool_use", 4096},
		{"", 16384},
		{"unknown", 16384},
	}

	for _, tt := range tests {
		t.Run(tt.taskType, func(t *testing.T) {
			got := selectBaseTokens(tt.taskType)
			if got != tt.want {
				t.Errorf("selectBaseTokens(%q) = %d, want %d", tt.taskType, got, tt.want)
			}
		})
	}
}
