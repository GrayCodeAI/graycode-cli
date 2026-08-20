package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAskUserQuestionTool_Metadata(t *testing.T) {
	tool := AskUserQuestionTool{}
	if tool.Name() != "AskUserQuestion" {
		t.Errorf("Name() = %q, want AskUserQuestion", tool.Name())
	}
	if len(tool.Aliases()) == 0 || tool.Aliases()[0] != "ask_user" {
		t.Errorf("Aliases() = %v, want ask_user", tool.Aliases())
	}
	params := tool.Parameters()
	if params["type"] != "object" {
		t.Errorf("Parameters() type = %v, want object", params["type"])
	}
}

func TestAskUserQuestionTool_Execute_SimpleQuestion(t *testing.T) {
	tool := AskUserQuestionTool{}
	var askedQuestion string

	tc := &ToolContext{
		AskUserFn: func(q string) (string, error) {
			askedQuestion = q
			return "User answer", nil
		},
	}
	ctx := WithToolContext(context.Background(), tc)

	input := json.RawMessage(`{"question":"Which database to use?"}`)
	res, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "User answer" {
		t.Errorf("res = %q, want 'User answer'", res)
	}
	if askedQuestion != "Which database to use?" {
		t.Errorf("askedQuestion = %q, want 'Which database to use?'", askedQuestion)
	}
}

func TestAskUserQuestionTool_Execute_WithOptions(t *testing.T) {
	tool := AskUserQuestionTool{}
	var askedQuestion string

	tc := &ToolContext{
		AskUserFn: func(q string) (string, error) {
			askedQuestion = q
			return "PostgreSQL", nil
		},
	}
	ctx := WithToolContext(context.Background(), tc)

	input := json.RawMessage(`{
		"question": "Which database?",
		"options": ["PostgreSQL", "MySQL", "SQLite"],
		"multi_select": false
	}`)
	res, err := tool.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "PostgreSQL" {
		t.Errorf("res = %q, want 'PostgreSQL'", res)
	}
	if !strings.Contains(askedQuestion, "Options:") || !strings.Contains(askedQuestion, "PostgreSQL") {
		t.Errorf("expected askedQuestion to contain options, got %q", askedQuestion)
	}
}

func TestAskUserQuestionTool_Execute_ValidationErrors(t *testing.T) {
	tool := AskUserQuestionTool{}

	// Invalid JSON
	_, err := tool.Execute(context.Background(), json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	// Empty question
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"question":""}`))
	if err == nil {
		t.Error("expected error for empty question")
	}

	// Unconfigured context
	_, err = tool.Execute(context.Background(), json.RawMessage(`{"question":"Test?"}`))
	if err == nil {
		t.Error("expected error when ask_user is not configured")
	}
}
