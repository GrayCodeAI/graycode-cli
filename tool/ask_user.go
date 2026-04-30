package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// AskUserFn is set by the TUI to handle user questions from the LLM.
var AskUserFn func(question string) (string, error)

type AskUserQuestionTool struct{}

func (AskUserQuestionTool) Name() string { return "ask_user" }
func (AskUserQuestionTool) Description() string {
	return "Ask the user a clarifying question. Use when you need more information to proceed."
}
func (AskUserQuestionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{"type": "string", "description": "The question to ask the user"},
		},
		"required": []string{"question"},
	}
}

func (AskUserQuestionTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Question string `json:"question"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if AskUserFn == nil {
		return "", fmt.Errorf("ask_user not configured")
	}
	return AskUserFn(p.Question)
}
