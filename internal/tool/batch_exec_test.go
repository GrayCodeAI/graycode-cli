package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBatchExecRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"submit","prompts":["hello"]}`,
	))
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchExecSubmitRequiresPrompts(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"submit","prompts":[]}`,
	))
	if err == nil || !strings.Contains(err.Error(), "at least one prompt") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchExecPollRequiresID(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"poll"}`,
	))
	if err == nil || !strings.Contains(err.Error(), "batch_id is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestBatchExecInvalidAction(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	_, err := (BatchExecTool{}).Execute(context.Background(), json.RawMessage(
		`{"action":"nope"}`,
	))
	if err == nil || !strings.Contains(err.Error(), "unsupported action") {
		t.Fatal("expected error for unsupported action")
	}
}
