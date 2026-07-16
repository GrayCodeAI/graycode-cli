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
			"options": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional list of choices (for single-select)",
			},
			"multi_select": map[string]interface{}{
				"type":        "boolean",
				"description": "Allow multiple selections (default: false)",
			},
			"other": map[string]interface{}{
				"type":        "boolean",
				"description": "Allow free-text 'other' option (default: true)",
			},
			"cancel_message": map[string]interface{}{
				"type":        "string",
				"description": "Message to show on cancel",
			},
		},
		"required": []string{"question"},
	}
}

// AskUserInput represents structured input for ask_user tool
type AskUserInput struct {
	Question      string   `json:"question"`
	Options       []string `json:"options,omitempty"`
	MultiSelect   bool     `json:"multi_select,omitempty"`
	Other         *bool    `json:"other,omitempty"`
	CancelMessage string   `json:"cancel_message,omitempty"`
}

func (AskUserQuestionTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p AskUserInput
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if p.Question == "" {
		return "", fmt.Errorf("question is required")
	}
	tc := GetToolContext(ctx)
	if tc == nil || tc.AskUserFn == nil {
		return "", fmt.Errorf("ask_user not configured")
	}
	if len(p.Options) > 0 {
		// Structured question with options
		return tc.AskUserFn(fmt.Sprintf("%s\nOptions: %v", p.Question, p.Options))
	}
	return tc.AskUserFn(p.Question)
}
