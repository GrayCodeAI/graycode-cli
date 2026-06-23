package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentTool_PromptTooLarge(t *testing.T) {
	t.Parallel()

	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			return prompt, nil
		},
	})
	oversized := strings.Repeat("a", maxAgentPromptBytes+1)

	_, err := AgentTool{}.Execute(ctx, mustJSONRaw(t, map[string]any{"prompt": oversized}))
	if err == nil || !strings.Contains(err.Error(), "agent prompt too large") {
		t.Fatalf("expected prompt-too-large error, got %v", err)
	}
}

func TestMultiAgentTool_TooManyTasks(t *testing.T) {
	t.Parallel()

	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			return prompt, nil
		},
	})
	tasks := make([]string, maxParallelAgentTasks+1)
	for i := range tasks {
		tasks[i] = "task"
	}

	_, err := MultiAgentTool{}.Execute(ctx, mustJSONRaw(t, map[string]any{"tasks": tasks}))
	if err == nil || !strings.Contains(err.Error(), "too many tasks") {
		t.Fatalf("expected too-many-tasks error, got %v", err)
	}
}

func TestMultiAgentTool_TaskPromptTooLarge(t *testing.T) {
	t.Parallel()

	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			return prompt, nil
		},
	})
	oversized := strings.Repeat("a", maxAgentPromptBytes+1)

	_, err := MultiAgentTool{}.Execute(ctx, mustJSONRaw(t, map[string]any{"tasks": []string{oversized}}))
	if err == nil || !strings.Contains(err.Error(), "task prompt too large") {
		t.Fatalf("expected task-prompt-too-large error, got %v", err)
	}
}

func mustJSONRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return data
}
