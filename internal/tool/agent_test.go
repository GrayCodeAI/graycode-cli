package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	agentcontracts "github.com/GrayCodeAI/eagle/agent"
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
		AgentSpawnFn: func(_ context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			return agentcontracts.SpawnResult{
				Status: agentcontracts.StatusCompleted,
				Output: "done:" + req.Prompt,
			}, nil
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
	if env.FullOutput != "done:task1" {
		t.Fatalf("unexpected envelope: %s", out)
	}
}

func TestAgentTool_TypedPlan(t *testing.T) {
	var gotType string
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			n, err := req.Normalize()
			if err != nil {
				return agentcontracts.SpawnResult{}, err
			}
			gotType = string(n.SubagentType)
			return agentcontracts.SpawnResult{Status: agentcontracts.StatusCompleted, Output: "plan"}, nil
		},
	})
	_, err := AgentTool{}.Execute(ctx, json.RawMessage(`{"prompt":"design API","subagent_type":"plan"}`))
	if err != nil {
		t.Fatal(err)
	}
	if gotType != string(agentcontracts.TypePlan) {
		t.Fatalf("subagent_type=%q want plan", gotType)
	}
}

func TestAgentTool_EmptyPromptRejected(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, _ agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			t.Fatal("spawn should not be called")
			return agentcontracts.SpawnResult{}, nil
		},
	})
	_, err := AgentTool{}.Execute(ctx, json.RawMessage(`{"prompt":""}`))
	if err == nil {
		t.Fatal("expected validation error for empty prompt")
	}
}

func TestMultiAgentTool_Success(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			return agentcontracts.SpawnResult{Output: "result:" + req.Prompt}, nil
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

func TestMultiAgentTool_TypedTasks(t *testing.T) {
	seen := make(map[string]bool)
	var mu sync.Mutex
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			n, _ := req.Normalize()
			mu.Lock()
			seen[string(n.SubagentType)] = true
			mu.Unlock()
			return agentcontracts.SpawnResult{Output: "ok"}, nil
		},
	})
	payload := `{"tasks":[{"prompt":"research","subagent_type":"explore"},{"prompt":"build","subagent_type":"general"}]}`
	if _, err := (MultiAgentTool{}).Execute(ctx, json.RawMessage(payload)); err != nil {
		t.Fatal(err)
	}
	if !seen["explore"] || !seen["general-purpose"] {
		t.Fatalf("types seen=%v want explore and general-purpose", seen)
	}
}

func TestMultiAgentTool_PartialError(t *testing.T) {
	ctx := WithToolContext(context.Background(), &ToolContext{
		AgentSpawnFn: func(_ context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
			if req.Prompt == "fail" {
				return agentcontracts.SpawnResult{}, fmt.Errorf("boom")
			}
			return agentcontracts.SpawnResult{Output: "ok"}, nil
		},
	})
	out, err := MultiAgentTool{}.Execute(ctx, json.RawMessage(`{"tasks":["pass","fail"]}`))
	if err != nil {
		t.Fatalf("expected nil error for partial failure, got %v", err)
	}
	if !strings.Contains(out, "ok") || !strings.Contains(out, "boom") {
		t.Fatalf("expected both success and error in output: %s", out)
	}
}
