package tool

import (
	"context"
	"fmt"
	"time"

	agentcontracts "github.com/GrayCodeAI/eagle/agent"

	"github.com/GrayCodeAI/graycode-cli/internal/taskruntime"
)

// BackgroundAgentManager tracks background sub-agent goroutines so the
// engine can wait for them and collect their results after the main LLM
// turn completes. It is a thin facade over taskruntime.Registry (PACK-02).
type BackgroundAgentManager struct {
	reg *taskruntime.Registry
}

// BackgroundAgent represents a running background sub-agent.
type BackgroundAgent struct {
	ID      string
	Prompt  string
	Request agentcontracts.SpawnRequest
	Started time.Time
}

// BackgroundResult holds the outcome of a completed background sub-agent.
type BackgroundResult struct {
	ID      string
	Prompt  string
	Request agentcontracts.SpawnRequest
	Output  string
	Result  agentcontracts.SpawnResult
	Err     error
	Done    time.Time
}

// NewBackgroundAgentManager creates a new manager backed by taskruntime.
func NewBackgroundAgentManager() *BackgroundAgentManager {
	return &BackgroundAgentManager{reg: taskruntime.New()}
}

// Registry exposes the underlying taskruntime registry (for Kill/Wait tools).
func (m *BackgroundAgentManager) Registry() *taskruntime.Registry {
	if m == nil {
		return nil
	}
	return m.reg
}

// Spawn starts a background sub-agent goroutine with a typed spawn request.
func (m *BackgroundAgentManager) Spawn(ctx context.Context, id string, req agentcontracts.SpawnRequest, spawnFn AgentSpawnFn) {
	if m == nil || m.reg == nil || spawnFn == nil {
		return
	}
	m.reg.SpawnAgent(ctx, id, req, taskruntime.SpawnFn(spawnFn))
}

// HasPending returns true if any background agents are still running.
func (m *BackgroundAgentManager) HasPending() bool {
	if m == nil || m.reg == nil {
		return false
	}
	return m.reg.HasPending()
}

// WaitForResults blocks until all pending agents complete or the timeout
// is reached. Returns all collected results (including any that completed
// before this call).
func (m *BackgroundAgentManager) WaitForResults(timeout time.Duration) []*BackgroundResult {
	if m == nil || m.reg == nil {
		return nil
	}
	tasks := m.reg.Wait(timeout)
	return tasksToResults(tasks)
}

// CollectResults returns and clears all completed results.
func (m *BackgroundAgentManager) CollectResults() []*BackgroundResult {
	if m == nil || m.reg == nil {
		return nil
	}
	return tasksToResults(m.reg.CollectCompleted())
}

// GetResult returns the result for a specific agent ID, if completed.
func (m *BackgroundAgentManager) GetResult(id string) (*BackgroundResult, bool) {
	if m == nil || m.reg == nil {
		return nil, false
	}
	t, ok := m.reg.Get(id)
	if !ok || t.Status == taskruntime.StatusRunning {
		return nil, false
	}
	return taskToResult(t), true
}

// IsRunning returns true if the agent with the given ID is still running.
func (m *BackgroundAgentManager) IsRunning(id string) bool {
	if m == nil || m.reg == nil {
		return false
	}
	return m.reg.IsRunning(id)
}

// Elapsed returns the elapsed time for a running agent.
func (m *BackgroundAgentManager) Elapsed(id string) time.Duration {
	if m == nil || m.reg == nil {
		return 0
	}
	return m.reg.Elapsed(id)
}

// Kill cancels a running background agent.
func (m *BackgroundAgentManager) Kill(id string) error {
	if m == nil || m.reg == nil {
		return fmt.Errorf("background manager not configured")
	}
	return m.reg.Kill(id)
}

// FormatResults returns a human-readable summary of background results
// suitable for injection into the conversation.
func FormatResults(results []*BackgroundResult) string {
	if len(results) == 0 {
		return ""
	}
	out := fmt.Sprintf("[Background sub-agent results: %d completed]\n\n", len(results))
	for _, r := range results {
		if r.Err != nil {
			out += fmt.Sprintf("--- Agent %s (error) ---\n%s\n\n", r.ID, r.Err.Error())
		} else {
			summary := r.Output
			if len(summary) > 500 {
				summary = summary[:500] + "...(truncated)"
			}
			out += fmt.Sprintf("--- Agent %s ---\n%s\n\n", r.ID, summary)
		}
	}
	return out
}

func tasksToResults(tasks []*taskruntime.Task) []*BackgroundResult {
	out := make([]*BackgroundResult, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskToResult(t))
	}
	return out
}

func taskToResult(t *taskruntime.Task) *BackgroundResult {
	br := &BackgroundResult{
		ID:      t.ID,
		Prompt:  t.Prompt,
		Request: t.Request,
		Output:  t.Output,
		Done:    t.DoneAt,
		Result: agentcontracts.SpawnResult{
			SubagentID: t.ID,
			Status:     string(t.Status),
			Output:     t.Output,
			Error:      t.Error,
		},
	}
	if t.Error != "" {
		br.Err = fmt.Errorf("%s", t.Error)
	}
	return br
}
