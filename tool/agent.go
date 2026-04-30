package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// AgentSpawnFn is set by the engine to spawn sub-agent sessions.
// It takes a prompt and returns the agent's response.
var AgentSpawnFn func(ctx context.Context, prompt string) (string, error)

type AgentTool struct{}

func (AgentTool) Name() string { return "agent" }
func (AgentTool) Description() string {
	return "Spawn a sub-agent to handle a complex task independently. Use for tasks that can be parallelized or require deep focus. The sub-agent has access to all tools."
}
func (AgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{"type": "string", "description": "Task description for the sub-agent"},
		},
		"required": []string{"prompt"},
	}
}

func (AgentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if AgentSpawnFn == nil {
		return "", fmt.Errorf("agent spawning not configured")
	}
	return AgentSpawnFn(ctx, p.Prompt)
}

// MultiAgentTool spawns multiple sub-agents in parallel.
type MultiAgentTool struct{}

func (MultiAgentTool) Name() string { return "multi_agent" }
func (MultiAgentTool) Description() string {
	return "Spawn multiple sub-agents in parallel. Each gets a separate task. Use when you need to do several independent things at once."
}
func (MultiAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tasks": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{"type": "string"},
				"description": "List of task prompts, one per sub-agent",
			},
		},
		"required": []string{"tasks"},
	}
}

func (MultiAgentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Tasks []string `json:"tasks"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	if AgentSpawnFn == nil {
		return "", fmt.Errorf("agent spawning not configured")
	}

	type result struct {
		idx    int
		output string
		err    error
	}
	results := make([]result, len(p.Tasks))
	var wg sync.WaitGroup
	for i, task := range p.Tasks {
		wg.Add(1)
		go func(idx int, prompt string) {
			defer wg.Done()
			out, err := AgentSpawnFn(ctx, prompt)
			results[idx] = result{idx: idx, output: out, err: err}
		}(i, task)
	}
	wg.Wait()

	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "=== Agent %d ===\n", i+1)
		if r.err != nil {
			fmt.Fprintf(&b, "Error: %s\n", r.err)
		} else {
			b.WriteString(r.output)
		}
		b.WriteString("\n\n")
	}
	return b.String(), nil
}
