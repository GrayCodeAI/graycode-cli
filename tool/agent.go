package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// backgroundCompletionTimeout is the maximum time to wait for background
// sub-agents in a single background-completion cycle.
const backgroundCompletionTimeout = 2 * time.Minute

type AgentTool struct{}

func (AgentTool) Name() string      { return "Agent" }
func (AgentTool) RiskLevel() string { return "medium" }
func (AgentTool) Aliases() []string { return []string{"agent", "Task"} }
func (AgentTool) Description() string {
	return "Spawn a sub-agent to handle a complex task independently. The sub-agent has access to all tools. Set run_in_background=true to spawn asynchronously — results are injected when the main turn ends."
}
func (AgentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prompt": map[string]interface{}{"type": "string", "description": "Task description for the sub-agent"},
			"run_in_background": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, spawn the sub-agent in the background and continue. Results are collected when the main turn ends.",
			},
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "ID of a previous sub-agent to resume. The sub-agent continues from where it left off with its full context preserved.",
			},
			"retry_of": map[string]interface{}{
				"type":        "string",
				"description": "ID of a failed sub-agent to retry. Spawns a new agent with the same task.",
			},
		},
		"required": []string{"prompt"},
	}
}

func (AgentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Prompt          string `json:"prompt"`
		RunInBackground bool   `json:"run_in_background"`
		AgentID         string `json:"agent_id"`
		RetryOf         string `json:"retry_of"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	tc := GetToolContext(ctx)
	if tc == nil || tc.AgentSpawnFn == nil {
		return "", fmt.Errorf("agent spawning not configured")
	}

	// Resume a previous agent by ID.
	if p.AgentID != "" {
		if tc.BackgroundManager == nil {
			return "", fmt.Errorf("background agent manager not configured")
		}
		result, ok := tc.BackgroundManager.GetResult(p.AgentID)
		if ok {
			return agentEnvelopeWithID(p.AgentID, "completed", result.Output), nil
		}
		if tc.BackgroundManager.IsRunning(p.AgentID) {
			elapsed := tc.BackgroundManager.Elapsed(p.AgentID)
			return fmt.Sprintf(`{"agent":"%s","status":"running","elapsed":"%s"}`, p.AgentID, elapsed), nil
		}
		return "", fmt.Errorf("agent_id %q not found", p.AgentID)
	}

	// Retry a failed agent.
	if p.RetryOf != "" {
		if tc.BackgroundManager == nil {
			return "", fmt.Errorf("background agent manager not configured")
		}
		result, ok := tc.BackgroundManager.GetResult(p.RetryOf)
		if !ok {
			return "", fmt.Errorf("retry_of %q not found or still running", p.RetryOf)
		}
		// Re-spawn with the original prompt.
		id := fmt.Sprintf("retry-%d", time.Now().UnixNano())
		tc.BackgroundManager.Spawn(ctx, id, result.Prompt, tc.AgentSpawnFn)
		return fmt.Sprintf(`{"agent":"%s","retry_of":"%s","status":"running","message":"Retrying failed agent."}`, id, p.RetryOf), nil
	}

	if p.RunInBackground {
		if tc.BackgroundManager == nil {
			return "", fmt.Errorf("background agent manager not configured")
		}
		id := fmt.Sprintf("bg-%d", time.Now().UnixNano())
		tc.BackgroundManager.Spawn(ctx, id, p.Prompt, tc.AgentSpawnFn)
		return fmt.Sprintf(`{"agent":"%s","status":"running","message":"Sub-agent spawned in background. Results will be injected when the main turn ends."}`, id), nil
	}

	out, err := tc.AgentSpawnFn(ctx, p.Prompt)
	if err != nil {
		return "", err
	}
	return agentEnvelope("success", out), nil
}

func agentEnvelope(status, output string) string {
	return agentEnvelopeWithID("sub-agent", status, output)
}

func agentEnvelopeWithID(id, status, output string) string {
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
		Agent:      id,
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
			"run_in_background": map[string]interface{}{
				"type":        "boolean",
				"description": "If true, spawn all sub-agents in the background.",
			},
		},
		"required": []string{"tasks"},
	}
}

func (MultiAgentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var p struct {
		Tasks           []string `json:"tasks"`
		RunInBackground bool     `json:"run_in_background"`
	}
	if err := json.Unmarshal(input, &p); err != nil {
		return "", err
	}
	tc := GetToolContext(ctx)
	if tc == nil || tc.AgentSpawnFn == nil {
		return "", fmt.Errorf("agent spawning not configured")
	}

	if p.RunInBackground {
		if tc.BackgroundManager == nil {
			return "", fmt.Errorf("background agent manager not configured")
		}
		ids := make([]string, len(p.Tasks))
		for i, task := range p.Tasks {
			id := fmt.Sprintf("bg-%d-%d", time.Now().UnixNano(), i)
			tc.BackgroundManager.Spawn(ctx, id, task, tc.AgentSpawnFn)
			ids[i] = id
		}
		b, _ := json.Marshal(ids)
		return fmt.Sprintf(`{"agents":%s,"status":"running","message":"%d sub-agents spawned in background."}`, string(b), len(ids)), nil
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
