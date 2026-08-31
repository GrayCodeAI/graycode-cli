// Package taskruntime is the unified registry for background agent and shell
// tasks (Year 0 PACK-02 + PACK-06).
package taskruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	agentcontracts "github.com/GrayCodeAI/eagle/agent"
)

// Kind classifies a background task.
type Kind string

const (
	KindAgent   Kind = "agent"
	KindShell   Kind = "shell"
	KindMonitor Kind = "monitor"
)

// Default is the process-wide registry used by shell background tasks and
// tools that do not carry a session-scoped BackgroundManager.
var Default = New()

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
// It does NOT hold the lock across the blocking wait, so other operations
// (SpawnAgent, Kill, Get, etc.) can proceed while waiting.
func (r *Registry) Wait(timeout time.Duration) []*Task {
	deadline := time.Now().Add(timeout)

	// cond.Wait() cannot be combined with a timeout directly, so a timer
	// goroutine broadcasts on the cond once the deadline elapses. A single
	// broadcast could be lost if it lands in the microsecond window between
	// loop iterations while the waiter is outside cond.Wait(), so the timer
	// repeats the broadcast every millisecond until Wait returns. The stop
	// channel terminates the goroutine when Wait returns (all tasks done or
	// deadline passed) so it cannot leak or fire after exit.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-time.After(timeout):
		case <-stop:
			return
		}
		for {
			r.cond.Broadcast()
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()

	for {
		r.mu.Lock()
		if len(r.running) == 0 {
			out := make([]*Task, 0, len(r.done))
			for _, t := range r.done {
				cp := *t
				out = append(out, &cp)
			}
			r.mu.Unlock()
			return out
		}
		if time.Now().After(deadline) {
			out := make([]*Task, 0, len(r.done))
			for _, t := range r.done {
				cp := *t
				out = append(out, &cp)
			}
			r.mu.Unlock()
			return out
		}
		// cond.Wait() atomically unlocks r.mu while blocking and re-locks on
		// wakeup, so other goroutines can acquire r.mu during the wait.
		r.cond.Wait()
		r.mu.Unlock()
	}
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

// RegisterExternal registers a running external task (e.g. shell process)
// so Wait/Kill/Get work through the same registry.
func (r *Registry) RegisterExternal(id string, kind Kind, label string, cancel context.CancelFunc) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running[id] = &Task{
		ID:        id,
		Kind:      kind,
		Prompt:    label,
		Status:    StatusRunning,
		StartedAt: time.Now(),
		cancel:    cancel,
	}
}

// FinishExternal marks an external task completed/failed/killed and moves it to done.
func (r *Registry) FinishExternal(id string, status Status, output, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.running[id]
	if !ok {
		// already finished or unknown
		if t, ok = r.done[id]; ok {
			t.Status = status
			t.Output = output
			t.Error = errMsg
			t.DoneAt = time.Now()
		}
		return
	}
	delete(r.running, id)
	t.Status = status
	t.Output = output
	t.Error = errMsg
	t.DoneAt = time.Now()
	r.done[id] = t
	r.cond.Broadcast()
}

// AppendOutput appends to a running or done task's output (monitor/shell).
func (r *Registry) AppendOutput(id, chunk string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.running[id]
	if !ok {
		t, ok = r.done[id]
		if !ok {
			return
		}
	}
	t.Output += chunk
	const max = 200_000
	if len(t.Output) > max {
		t.Output = "...(truncated)\n" + t.Output[len(t.Output)-max:]
	}
}

// WaitIDs blocks until all listed task ids are not running, or timeout.
// Like Wait, it does not hold the lock across the blocking wait.
func (r *Registry) WaitIDs(ids []string, timeout time.Duration) []*Task {
	if len(ids) == 0 {
		return r.Wait(timeout)
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	deadline := time.Now().Add(timeout)

	// cond.Wait() cannot be combined with a timeout directly, so a timer
	// goroutine broadcasts on the cond once the deadline elapses. A single
	// broadcast could be lost if it lands in the microsecond window between
	// loop iterations while the waiter is outside cond.Wait(), so the timer
	// repeats the broadcast every millisecond until WaitIDs returns. The stop
	// channel terminates the goroutine when WaitIDs returns so it cannot leak
	// or fire after exit.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-time.After(timeout):
		case <-stop:
			return
		}
		for {
			r.cond.Broadcast()
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()

	for {
		r.mu.Lock()
		still := false
		for id := range want {
			if _, ok := r.running[id]; ok {
				still = true
				break
			}
		}
		if !still || time.Now().After(deadline) {
			out := make([]*Task, 0, len(ids))
			for _, id := range ids {
				if t, ok := r.done[id]; ok {
					cp := *t
					out = append(out, &cp)
				} else if t, ok := r.running[id]; ok {
					cp := *t
					out = append(out, &cp)
				}
			}
			r.mu.Unlock()
			return out
		}
		// cond.Wait() atomically unlocks r.mu while blocking.
		r.cond.Wait()
		r.mu.Unlock()
	}
}

// List returns snapshots of all running and recently completed tasks.
func (r *Registry) List() []*Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Task, 0, len(r.running)+len(r.done))
	for _, t := range r.running {
		cp := *t
		out = append(out, &cp)
	}
	for _, t := range r.done {
		cp := *t
		out = append(out, &cp)
	}
	return out
}
