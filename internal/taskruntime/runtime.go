// Package taskruntime is the unified registry for background agent tasks
// (Year 0 PACK-02). Shell background tasks remain in tool/task_tools.go for
// now and will merge under this registry in PACK-06.
package taskruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	agentcontracts "github.com/GrayCodeAI/hawk-core-contracts/agent"
)

// Kind classifies a background task.
type Kind string

const (
	KindAgent Kind = "agent"
	KindShell Kind = "shell" // reserved for PACK-06 merge
)

// Status is a task lifecycle state.
type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusKilled    Status = "killed"
)

// Task is a registry entry for a background unit of work.
type Task struct {
	ID        string
	Kind      Kind
	Prompt    string
	Request   agentcontracts.SpawnRequest
	Status    Status
	Output    string
	Error     string
	StartedAt time.Time
	DoneAt    time.Time

	cancel context.CancelFunc
}

// SpawnFn runs a typed agent spawn (same contract as tool.AgentSpawnFn).
type SpawnFn func(ctx context.Context, req agentcontracts.SpawnRequest) (agentcontracts.SpawnResult, error)

// Registry is the process-local task store.
type Registry struct {
	mu      sync.Mutex
	cond    *sync.Cond
	seq     int
	running map[string]*Task
	done    map[string]*Task
}

// New creates an empty registry.
func New() *Registry {
	r := &Registry{
		running: make(map[string]*Task),
		done:    make(map[string]*Task),
	}
	r.cond = sync.NewCond(&r.mu)
	return r
}

// SpawnAgent starts a background agent task and returns its id immediately.
func (r *Registry) SpawnAgent(parent context.Context, id string, req agentcontracts.SpawnRequest, fn SpawnFn) string {
	if fn == nil {
		return ""
	}
	if id == "" {
		r.mu.Lock()
		r.seq++
		id = fmt.Sprintf("agent-%d", r.seq)
		r.mu.Unlock()
	}
	ctx, cancel := context.WithCancel(parent)
	task := &Task{
		ID:        id,
		Kind:      KindAgent,
		Prompt:    req.Prompt,
		Request:   req,
		Status:    StatusRunning,
		StartedAt: time.Now(),
		cancel:    cancel,
	}
	r.mu.Lock()
	r.running[id] = task
	r.mu.Unlock()

	go func() {
		res, err := fn(ctx, req)
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.running, id)
		task.DoneAt = time.Now()
		if ctx.Err() == context.Canceled && err == nil && res.Output == "" {
			task.Status = StatusKilled
			task.Error = "killed"
		} else if err != nil {
			task.Status = StatusFailed
			task.Error = err.Error()
			task.Output = res.Output
		} else {
			task.Status = StatusCompleted
			task.Output = res.Output
			if task.Output == "" {
				task.Output = res.Summary
			}
		}
		r.done[id] = task
		r.cond.Broadcast()
	}()
	return id
}

// Get returns a task by id (running or completed).
func (r *Registry) Get(id string) (*Task, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.running[id]; ok {
		cp := *t
		return &cp, true
	}
	if t, ok := r.done[id]; ok {
		cp := *t
		return &cp, true
	}
	return nil, false
}

// IsRunning reports whether id is still executing.
func (r *Registry) IsRunning(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.running[id]
	return ok
}

// Elapsed returns runtime for a running task.
func (r *Registry) Elapsed(id string) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.running[id]; ok {
		return time.Since(t.StartedAt)
	}
	return 0
}

// Kill cancels a running task.
func (r *Registry) Kill(id string) error {
	r.mu.Lock()
	t, ok := r.running[id]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("task %q not running", id)
	}
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// Wait blocks until no tasks are running or timeout elapses.
func (r *Registry) Wait(timeout time.Duration) []*Task {
	deadline := time.Now().Add(timeout)
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.running) > 0 {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		timer := time.AfterFunc(remaining, func() {
			r.mu.Lock()
			r.cond.Broadcast()
			r.mu.Unlock()
		})
		r.cond.Wait()
		timer.Stop()
	}
	out := make([]*Task, 0, len(r.done))
	for _, t := range r.done {
		cp := *t
		out = append(out, &cp)
	}
	return out
}

// CollectCompleted returns and clears completed tasks.
func (r *Registry) CollectCompleted() []*Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Task, 0, len(r.done))
	for id, t := range r.done {
		cp := *t
		out = append(out, &cp)
		delete(r.done, id)
	}
	return out
}

// PendingCount returns running task count.
func (r *Registry) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.running)
}

// HasPending reports whether any task is running.
func (r *Registry) HasPending() bool {
	return r.PendingCount() > 0
}
