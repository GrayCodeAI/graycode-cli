package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type AgentTool struct{}

func (AgentTool) Name() string      { return "Agent" }
func (AgentTool) Aliases() []string { return []string{"agent", "Task"} }
func (AgentTool) Description() string {
	return "Spawn a sub-agent to handle a complex task independently. The sub-agent has access to all tools."
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
	tc := GetToolContext(ctx)
	if tc == nil || tc.AgentSpawnFn == nil {
		return "", fmt.Errorf("agent spawning not configured")
	}
	return tc.AgentSpawnFn(ctx, p.Prompt)
}

// MultiAgentTool spawns multiple sub-agents in parallel.
type MultiAgentTool struct{}

func (MultiAgentTool) Name() string      { return "MultiAgent" }
func (MultiAgentTool) Aliases() []string { return []string{"multi_agent"} }
func (MultiAgentTool) Description() string {
	return "Spawn multiple sub-agents in parallel for independent tasks."
}
func (MultiAgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tasks": map[string]interface{}{
				"type":  "array",
				"items": map[string]interface{}{"type": "string"},
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
	tc := GetToolContext(ctx)
	if tc == nil || tc.AgentSpawnFn == nil {
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
			out, err := tc.AgentSpawnFn(ctx, prompt)
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
