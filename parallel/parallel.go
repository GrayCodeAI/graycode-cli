package parallel

import (
	"context"
	"fmt"
	"sync"
)

// Status represents the current state of a parallel task.
type Status int

const (
	StatusPending Status = iota
	StatusRunning
	StatusDone
	StatusFailed
)

// String returns a human-readable label for the status.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Task represents a single unit of work that will execute in its own git worktree.
type Task struct {
	ID           string
	Description  string
	Branch       string // auto-generated branch name: hawk-parallel/{ID}
	WorktreePath string // filesystem path to the worktree
	Status       Status
	Error        error
	Result       string // summary of what was done
}

// Pool manages parallel task execution using isolated git worktrees.
type Pool struct {
	repoDir    string
	baseBranch string
	tasks      []*Task
	maxWorkers int
	mu         sync.Mutex
	nextID     int
}

// NewPool creates a parallel execution pool rooted at the given git repo.
// maxWorkers caps the number of concurrent goroutines; if <= 0 it defaults to 4.
func NewPool(repoDir string, baseBranch string, maxWorkers int) *Pool {
	if maxWorkers <= 0 {
		maxWorkers = 4
	}
	return &Pool{
		repoDir:    repoDir,
		baseBranch: baseBranch,
		maxWorkers: maxWorkers,
	}
}

// AddTask queues a task for parallel execution and returns it.
// The task ID and branch name are auto-generated.
func (p *Pool) AddTask(description string) *Task {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.nextID++
	id := fmt.Sprintf("task-%d", p.nextID)
	t := &Task{
		ID:          id,
		Description: description,
		Branch:      fmt.Sprintf("hawk-parallel/%s", id),
		Status:      StatusPending,
	}
	p.tasks = append(p.tasks, t)
	return t
}

// Run executes all queued tasks in parallel, each in its own git worktree.
// workFn receives the worktree path and task, and returns a result summary.
// Tasks that fail do not prevent other tasks from completing.
func (p *Pool) Run(ctx context.Context, workFn func(ctx context.Context, worktreePath string, task *Task) (string, error)) error {
	p.mu.Lock()
	tasks := make([]*Task, len(p.tasks))
	copy(tasks, p.tasks)
	p.mu.Unlock()

	if len(tasks) == 0 {
		return nil
	}

	// Determine effective concurrency: min(len(tasks), maxWorkers).
	workers := len(tasks)
	if workers > p.maxWorkers {
		workers = p.maxWorkers
	}

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for _, t := range tasks {
		wg.Add(1)
		go func(task *Task) {
			defer wg.Done()

			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				p.mu.Lock()
				task.Status = StatusFailed
				task.Error = ctx.Err()
				p.mu.Unlock()
				return
			}
			defer func() { <-sem }()

			p.mu.Lock()
			task.Status = StatusRunning
			p.mu.Unlock()

			// Create worktree.
			wtPath, err := createWorktree(p.repoDir, p.baseBranch, task.Branch)
			if err != nil {
				p.mu.Lock()
				task.Status = StatusFailed
				task.Error = fmt.Errorf("create worktree: %w", err)
				p.mu.Unlock()
				return
			}

			p.mu.Lock()
			task.WorktreePath = wtPath
			p.mu.Unlock()

			// Execute the work function.
			result, err := workFn(ctx, wtPath, task)

			p.mu.Lock()
			if err != nil {
				task.Status = StatusFailed
				task.Error = err
			} else {
				task.Status = StatusDone
				task.Result = result
			}
			p.mu.Unlock()
		}(t)
	}

	wg.Wait()
	return nil
}

// Results returns all tasks (completed, failed, or pending).
func (p *Pool) Results() []*Task {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]*Task, len(p.tasks))
	copy(out, p.tasks)
	return out
}

// Cleanup removes all worktrees created by this pool. It is idempotent.
func (p *Pool) Cleanup() error {
	p.mu.Lock()
	tasks := make([]*Task, len(p.tasks))
	copy(tasks, p.tasks)
	p.mu.Unlock()

	var firstErr error
	for _, t := range tasks {
		if t.WorktreePath == "" {
			continue
		}
		if err := removeWorktree(p.repoDir, t.WorktreePath); err != nil && firstErr == nil {
			firstErr = err
		}
		// Clear the path so repeated Cleanup calls are safe.
		p.mu.Lock()
		t.WorktreePath = ""
		p.mu.Unlock()
	}
	return firstErr
}
