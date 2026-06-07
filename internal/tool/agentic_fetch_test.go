package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAgenticFetch_BatchFanOut(t *testing.T) {
	var calls int32
	tc := &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			atomic.AddInt32(&calls, 1)
			// Echo back which URL this invocation was for so we can assert
			// each URL produced its own labeled section.
			for _, u := range []string{"https://a.example", "https://b.example", "https://c.example"} {
				if strings.Contains(prompt, u) {
					return "summary-of " + u, nil
				}
			}
			return "summary", nil
		},
	}
	ctx := WithToolContext(context.Background(), tc)

	input, _ := json.Marshal(map[string]any{
		"urls":  []string{"https://a.example", "https://b.example", "https://c.example"},
		"query": "what is this",
	})
	out, err := (AgenticFetchTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("AgentSpawnFn called %d times, want 3", got)
	}
	for _, u := range []string{"https://a.example", "https://b.example", "https://c.example"} {
		if !strings.Contains(out, "## "+u) {
			t.Errorf("output missing labeled section for %q\n%s", u, out)
		}
		if !strings.Contains(out, "summary-of "+u) {
			t.Errorf("output missing summary for %q", u)
		}
	}
}

func TestAgenticFetch_SingleURLBackCompat(t *testing.T) {
	var gotPrompt string
	tc := &ToolContext{
		AgentSpawnFn: func(_ context.Context, prompt string) (string, error) {
			gotPrompt = prompt
			return "ok", nil
		},
	}
	ctx := WithToolContext(context.Background(), tc)

	input, _ := json.Marshal(map[string]any{"url": "https://only.example", "query": "q"})
	out, err := (AgenticFetchTool{}).Execute(ctx, input)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Single URL keeps the bare sub-agent output (no "## url" batch header).
	if out != "ok" {
		t.Errorf("single-url output = %q, want %q", out, "ok")
	}
	// The refusal contract must be present in the prompt.
	if !strings.Contains(gotPrompt, "NO_RELEVANT_INFORMATION") {
		t.Error("research prompt missing relevance-refusal contract")
	}
}

func TestAgenticFetch_RequiresURL(t *testing.T) {
	input, _ := json.Marshal(map[string]any{"query": "q"})
	if _, err := (AgenticFetchTool{}).Execute(context.Background(), input); err == nil {
		t.Error("expected error when neither url nor urls is provided")
	}
}

func TestWebSearch_QueryNormalization(t *testing.T) {
	// Neither query nor queries → error, without touching the network.
	input, _ := json.Marshal(map[string]any{"numResults": 3})
	if _, err := (WebSearchTool{}).Execute(context.Background(), input); err == nil {
		t.Error("expected error when neither query nor queries is provided")
	}
}
