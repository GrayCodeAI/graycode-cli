package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

type AskUserQuestionTool struct{}

func (AskUserQuestionTool) Name() string      { return "AskUserQuestion" }
func (AskUserQuestionTool) Aliases() []string { return []string{"ask_user"} }
func (AskUserQuestionTool) Description() string {
	return "Ask the user a clarifying question when you need more information to proceed."
}
func (AskUserQuestionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"question": map[string]interface{}{"type": "string", "description": "The question to ask"},
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
	tc := GetToolContext(ctx)
	if tc == nil || tc.AskUserFn == nil {
		return "", fmt.Errorf("ask_user not configured")
	}
	return tc.AskUserFn(p.Question)
}
