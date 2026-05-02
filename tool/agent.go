package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

type AgentTool struct{}

func (AgentTool) Name() string      { return "Agent" }
func (AgentTool) RiskLevel() string { return "medium" }
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
	out, err := tc.AgentSpawnFn(ctx, p.Prompt)
	if err != nil {
		return "", err
	}
	return agentEnvelope("success", out), nil
}

func agentEnvelope(status, output string) string {
	summary := output
	if len(summary) > 200 {
		summary = summary[:200]
	}
	env := struct {
		Agent      string `json:"agent"`
		Status     string `json:"status"`
		Summary    string `json:"summary"`
		TokensUsed int    `json:"tokens_used"`
		FullOutput string `json:"full_output"`
	}{
		Agent:      "sub-agent",
		Status:     status,
		Summary:    summary,
		TokensUsed: 0,
		FullOutput: output,
	}
	b, _ := json.Marshal(env)
	return string(b)
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
	envelopes := make([]json.RawMessage, len(results))
	for i, r := range results {
		var status, output string
		if r.err != nil {
			status = "error"
			output = r.err.Error()
		} else {
			status = "success"
			output = r.output
		}
		envelopes[i] = json.RawMessage(agentEnvelope(status, output))
	}
	b, _ := json.Marshal(envelopes)
	return string(b), nil
}
