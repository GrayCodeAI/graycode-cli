package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAgentTool_NoContext(t *testing.T) {
	_, err := AgentTool{}.Execute(context.Background(), json.RawMessage(`{"prompt":"hi"}`))
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected 'not configured' error, got %v", err)
	}
}

func TestAgentTool_NoSpawnFn(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{})
	_, err := AgentTool{}.Execute(ctx, json.RawMessage(`{"prompt":"hi"}`))
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected 'not configured' error, got %v", err)
	}
}

func TestAgentTool_Success(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			return "done:" + prompt, nil
		},
	})
	out, err := AgentTool{}.Execute(ctx, json.RawMessage(`{"prompt":"task1"}`))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Agent      string `json:"agent"`
		Status     string `json:"status"`
		Summary    string `json:"summary"`
		FullOutput string `json:"full_output"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("expected JSON envelope, got parse error: %v", err)
	}
	if env.Agent != "sub-agent" || env.Status != "success" || env.FullOutput != "done:task1" {
		t.Fatalf("unexpected envelope: %s", out)
	}
}

func TestAgentTool_EmptyPrompt(t *testing.T) {
	called := false
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, _ string) (string, error) {
			called = true
			return "ok", nil
		},
	})
	_, err := AgentTool{}.Execute(ctx, json.RawMessage(`{"prompt":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("spawn fn was not called for empty prompt")
	}
}

func TestMultiAgentTool_Success(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			return "result:" + prompt, nil
		},
	})
	out, err := MultiAgentTool{}.Execute(ctx, json.RawMessage(`{"tasks":["a","b"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var envelopes []json.RawMessage
	if err := json.Unmarshal([]byte(out), &envelopes); err != nil {
		t.Fatalf("expected JSON array, got parse error: %v", err)
	}
	if len(envelopes) != 2 {
		t.Fatalf("expected 2 envelopes, got %d", len(envelopes))
	}
	if !strings.Contains(out, "result:a") || !strings.Contains(out, "result:b") {
		t.Fatalf("missing results in output: %s", out)
	}
}

func TestMultiAgentTool_PartialError(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			if prompt == "fail" {
				return "", fmt.Errorf("boom")
			}
			return "ok", nil
		},
	})
	out, err := MultiAgentTool{}.Execute(ctx, json.RawMessage(`{"tasks":["pass","fail"]}`))
	if err != nil {
		t.Fatalf("expected nil error for partial failure, got %v", err)
	}
	if !strings.Contains(out, "ok") || !strings.Contains(out, "boom") {
		t.Fatalf("expected both success and error in output: %s", out)
	}
	var envelopes []json.RawMessage
	if err := json.Unmarshal([]byte(out), &envelopes); err != nil {
		t.Fatalf("expected JSON array, got parse error: %v", err)
	}
}
