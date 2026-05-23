package engine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BackgroundTask represents an async subagent task running in the background.
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
type BackgroundRunner struct {
	mu    sync.Mutex
	tasks map[string]*BackgroundTask
	seq   int
}

// NewBackgroundRunner creates a new background task runner.
func NewBackgroundRunner() *BackgroundRunner {
	return &BackgroundRunner{tasks: make(map[string]*BackgroundTask)}
}

// Delegate starts a background task. Returns the task ID immediately.
func (br *BackgroundRunner) Delegate(ctx context.Context, prompt string, execFn func(context.Context, string) (string, error)) string {
	br.mu.Lock()
	br.seq++
	id := fmt.Sprintf("bg-%d", br.seq)
	task := &BackgroundTask{
		ID:        id,
		Prompt:    prompt,
		Status:    "running",
		StartedAt: time.Now(),
	}
	br.tasks[id] = task
	br.mu.Unlock()

	go func() {
		taskCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		result, err := execFn(taskCtx, prompt)
		br.mu.Lock()
		defer br.mu.Unlock()
		task.DoneAt = time.Now()
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
		} else {
			task.Status = "done"
			task.Result = result
		}
	}()

	return id
}

// Status returns the current state of a background task.
func (br *BackgroundRunner) Status(id string) *BackgroundTask {
	br.mu.Lock()
	defer br.mu.Unlock()
	return br.tasks[id]
}

// Collect returns and removes a completed task's result. Returns nil if still running.
func (br *BackgroundRunner) Collect(id string) *BackgroundTask {
	br.mu.Lock()
	defer br.mu.Unlock()
	task, ok := br.tasks[id]
	if !ok {
		return nil
	}
	if task.Status == "running" {
		return nil
	}
	delete(br.tasks, id)
	return task
}

// ListActive returns all currently running tasks.
func (br *BackgroundRunner) ListActive() []*BackgroundTask {
	br.mu.Lock()
	defer br.mu.Unlock()
	var active []*BackgroundTask
	for _, t := range br.tasks {
		active = append(active, t)
	}
	return active
}

// PendingCount returns the number of tasks still running.
func (br *BackgroundRunner) PendingCount() int {
	br.mu.Lock()
	defer br.mu.Unlock()
	count := 0
	for _, t := range br.tasks {
		if t.Status == "running" {
			count++
		}
	}
	return count
}
