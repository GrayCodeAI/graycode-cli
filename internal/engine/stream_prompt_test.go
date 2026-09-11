package engine

import (
	"testing"

	"github.com/GrayCodeAI/hawk/internal/types"
)

func TestIsSmallTalkPrompt(t *testing.T) {
	tests := []struct {
		prompt string
		skip   bool
	}{
		{prompt: "Hi", skip: true},
		{prompt: "Hello!", skip: true},
		{prompt: "what can you do?", skip: true},
		{prompt: "how's it going", skip: true},
		{prompt: "good morning", skip: true},
		{prompt: "thanks", skip: true},
		{prompt: "Hi, inspect this repository", skip: false},
		{prompt: "run the tests", skip: false},
		{prompt: "hello there, who fixes this bug?", skip: false},
	}
	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			if got := isSmallTalkPrompt(tt.prompt); got != tt.skip {
				t.Fatalf("isSmallTalkPrompt(%q) = %v, want %v", tt.prompt, got, tt.skip)
			}
		})
	}
}

func TestSessionHasToolUse(t *testing.T) {
	plain := []types.EyrieMessage{{Role: "user", Content: "hi"}}
	used := []types.EyrieMessage{
		{Role: "user", Content: "read stream.go"},
		{Role: "assistant", ToolUse: []types.ToolCall{{Name: "Read"}}},
		{Role: "user", Content: "thanks", ToolResults: []types.ToolResult{{}}},
	}
	if sessionHasToolUse(plain) {
		t.Fatal("sessionHasToolUse(plain) = true, want false")
	}
	if !sessionHasToolUse(used) {
		t.Fatal("sessionHasToolUse(used) = false, want true")
	}
}
