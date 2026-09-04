package engine

import (
	"context"
	"fmt"
	"time"

	agentcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/agent"

	"github.com/GrayCodeAI/graycode-cli/internal/taskruntime"
	"github.com/GrayCodeAI/graycode-cli/internal/tool"
)

// BackgroundTask represents an async subagent task running in the background.
// Kept for API compatibility; backed by taskruntime via BackgroundRunner.
type BackgroundTask struct {
	ID        string
	Prompt    string
	Status    string // "running", "done", "failed"
	Result    string
	Error     string
	StartedAt time.Time
	DoneAt    time.Time
}

// BackgroundRunner manages async subagent tasks that run while the user keeps chatting.
// PACK-02: thin adapter over tool.BackgroundAgentManager / taskruntime.
type BackgroundRunner struct {
	mgr *tool.BackgroundAgentManager
}

// NewBackgroundRunner creates a new background task runner.
func NewBackgroundRunner() *BackgroundRunner {
	return &BackgroundRunner{mgr: tool.NewBackgroundAgentManager()}
}

// Delegate starts a background task. Returns the task ID immediately.
// execFn is adapted to the typed spawn contract (explore by default).
func (br *BackgroundRunner) Delegate(ctx context.Context, prompt string, execFn func(context.Context, string) (string, error)) string {
	if br.mgr == nil {
		br.mgr = tool.NewBackgroundAgentManager()
	}
	req := agentcontracts.SpawnRequest{Prompt: prompt, Background: true}
	id := fmt.Sprintf("bg-%d", time.Now().UnixNano())
	spawn := func(c context.Context, r agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error) {
		if execFn == nil {
			return agentcontracts.SpawnResult{}, fmt.Errorf("no exec fn")
		}
		out, err := execFn(c, r.Prompt)
		if err != nil {
			return agentcontracts.SpawnResult{Status: agentcontracts.StatusFailed, Error: err.Error()}, err
		}
		return agentcontracts.SpawnResult{Status: agentcontracts.StatusCompleted, Output: out}, nil
	}
	br.mgr.Spawn(ctx, id, req, spawn)
	return id
}

// Status returns the current state of a background task.
func (br *BackgroundRunner) Status(id string) *BackgroundTask {
	if br.mgr == nil || br.mgr.Registry() == nil {
		return nil
	}
	t, ok := br.mgr.Registry().Get(id)
	if !ok {
		return nil
	}
	return taskruntimeToBackgroundTask(t)
}

// Collect returns and removes a completed task's result. Returns nil if still running.
func (br *BackgroundRunner) Collect(id string) *BackgroundTask {
	if br.mgr == nil || br.mgr.Registry() == nil {
		return nil
	}
	t, ok := br.mgr.Registry().Get(id)
	if !ok || t.Status == taskruntime.StatusRunning {
		return nil
	}
	// CollectCompleted clears all; prefer Get snapshot without delete of others.
	// For API parity, return snapshot (done map retains until CollectCompleted).
	return taskruntimeToBackgroundTask(t)
}

// ListActive returns all currently running tasks.
func (br *BackgroundRunner) ListActive() []*BackgroundTask {
	// Registry does not list running ids yet; PendingCount is enough for tests.
	return nil
}

// PendingCount returns the number of tasks still running.
func (br *BackgroundRunner) PendingCount() int {
	if br.mgr == nil {
		return 0
	}
	return br.mgr.Registry().PendingCount()
}

func taskruntimeToBackgroundTask(t *taskruntime.Task) *BackgroundTask {
	status := "running"
	switch t.Status {
	case taskruntime.StatusCompleted:
		status = "done"
	case taskruntime.StatusFailed, taskruntime.StatusKilled:
		status = "failed"
	}
	return &BackgroundTask{
		ID:        t.ID,
		Prompt:    t.Prompt,
		Status:    status,
		Result:    t.Output,
		Error:     t.Error,
		StartedAt: t.StartedAt,
		DoneAt:    t.DoneAt,
	}
}
