package tool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TaskExecutorFunc runs a single task and returns its result output. The
// context is cancelled when the run is stopped or the per-task timeout elapses;
// implementations should respect it.
type TaskExecutorFunc func(ctx context.Context, task *Task) (string, error)

// TaskRunnerEvent is emitted through OnProgress when a task's lifecycle state
// changes. Events: "started", "completed", "retrying", "replanned", "failed",
// "skipped", "cancelled".
type TaskRunnerEvent string

const (
	EventStarted   TaskRunnerEvent = "started"
	EventCompleted TaskRunnerEvent = "completed"
	EventRetrying  TaskRunnerEvent = "retrying"
	EventReplanned TaskRunnerEvent = "replanned"
	EventFailed    TaskRunnerEvent = "failed"
	EventSkipped   TaskRunnerEvent = "skipped"
	EventCancelled TaskRunnerEvent = "cancelled"
)

// TaskRunnerOptions configures the TaskRunner. Zero values fall back to sane
// defaults; only Store and Execute are required.
type TaskRunnerOptions struct {
	// Store is the task store the runner drives. Required.
	Store *TaskStore
	// Execute runs one task and returns its output. Required.
	Execute TaskExecutorFunc
	// OnProgress is called (outside locks) on each lifecycle transition.
	OnProgress func(task *Task, event TaskRunnerEvent)
	// OnReplan is invoked when a task parks failed (retry budget exhausted).
	// Return true to requeue the task with a fresh retry budget (the
	// "replan remaining work" behavior). Bounded by MaxReplans per task.
	OnReplan func(ctx context.Context, task *Task) (bool, error)
	// MaxTotalTasks is the watchdog cap on distinct tasks that may reach a
	// terminal state in one run (default 50). Guards against unbounded work.
	MaxTotalTasks int
	// MaxReplans caps replans per task (default 2).
	MaxReplans int
	// PollInterval is the idle poll period (default 200ms).
	PollInterval time.Duration
	// Concurrency caps parallel task executions (default 1).
	Concurrency int
	// Backoff returns the wait before retrying attempt n (default quadratic,
	// capped at 30s).
	Backoff func(attempt int) time.Duration
	// DefaultTimeout bounds each task execution (0 = no timeout).
	DefaultTimeout time.Duration
}

// TaskRunnerStats is a point-in-time snapshot of runner activity.
type TaskRunnerStats struct {
	Running   bool
	Executed  int
	Completed int
	Failed    int
	Skipped   int
	Cancelled int
	Replanned int
}

const (
	defaultMaxTotalTasks = 50
	defaultMaxReplans    = 2
	defaultPollInterval  = 200 * time.Millisecond
	defaultConcurrency   = 1
	defaultBackoffCap    = 30 * time.Second
)

// TaskRunner is a background executor that drives a TaskStore: it repeatedly
// picks up ready work (pending tasks with no open blockers), executes each task
// through Execute, applies the store's retry budget on failure (with backoff),
// replans tasks that exhaust their budget, and stops under a watchdog cap or on
// cancellation. It is the execution half of the store-only TaskStore.
type TaskRunner struct {
	mu      sync.Mutex
	store   *TaskStore
	opts    TaskRunnerOptions
	backoff func(attempt int) time.Duration

	started  bool
	finished bool
	cancel   context.CancelFunc
	doneCh   chan struct{}
	runErr   error

	replansByTask map[string]int

	executed  int
	completed int
	failed    int
	skipped   int
	cancelled int
	replanned int
}

// NewTaskRunner validates and fills options with defaults.
func NewTaskRunner(opts TaskRunnerOptions) *TaskRunner {
	if opts.MaxTotalTasks <= 0 {
		opts.MaxTotalTasks = defaultMaxTotalTasks
	}
	if opts.MaxReplans < 0 {
		opts.MaxReplans = 0
	}
	if opts.MaxReplans == 0 {
		opts.MaxReplans = defaultMaxReplans
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultPollInterval
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaultConcurrency
	}
	if opts.Backoff == nil {
		opts.Backoff = defaultBackoff
	}
	return &TaskRunner{
		store:         opts.Store,
		opts:          opts,
		backoff:       opts.Backoff,
		replansByTask: make(map[string]int),
	}
}

func defaultBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	d := time.Duration(attempt) * time.Duration(attempt) * 500 * time.Millisecond
	if d > defaultBackoffCap {
		d = defaultBackoffCap
	}
	return d
}

// Run drives the store until quiescence (no ready work and nothing in flight),
// the watchdog cap, or ctx cancellation — whichever comes first. It is safe to
// call directly (blocking) or via Start.
func (r *TaskRunner) Run(ctx context.Context) error {
	if r == nil || r.store == nil {
		return fmt.Errorf("task runner: store is required")
	}
	if r.opts.Execute == nil {
		return fmt.Errorf("task runner: execute function is required")
	}

	ticker := time.NewTicker(r.opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.cancelActive("cancelled")
			return nil
		default:
		}

		if r.terminalCount() >= r.opts.MaxTotalTasks {
			return nil
		}

		ready := r.store.GetReadyWork()
		if len(ready) == 0 {
			if r.quiescent() {
				return nil
			}
			select {
			case <-ctx.Done():
				r.cancelActive("cancelled")
				return nil
			case <-ticker.C:
			}
			continue
		}

		due := r.dueTasks(ready, time.Now())
		if len(due) == 0 {
			select {
			case <-ctx.Done():
				r.cancelActive("cancelled")
				return nil
			case <-ticker.C:
			}
			continue
		}

		r.runBatch(ctx, due)
	}
}

// runBatch executes the due tasks concurrently up to Concurrency. Tasks that
// become non-pending while queued are skipped by executeOne's re-check.
func (r *TaskRunner) runBatch(ctx context.Context, tasks []*Task) {
	sem := make(chan struct{}, r.opts.Concurrency)
	var wg sync.WaitGroup
	for _, task := range tasks {
		if r.terminalCount() >= r.opts.MaxTotalTasks {
			break
		}
		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(t *Task) {
			defer wg.Done()
			defer func() { <-sem }()
			r.executeOne(ctx, t)
		}(task)
	}
	wg.Wait()
}

func (r *TaskRunner) executeOne(ctx context.Context, task *Task) {
	// Watchdog: never start work once the terminal-state cap is reached. This
	// guard lives here (not just at batch build time) because goroutines are
	// queued before earlier ones complete.
	if r.terminalCount() >= r.opts.MaxTotalTasks {
		return
	}
	cur, ok := r.store.Get(task.ID)
	if !ok || cur.Status != TaskStatusPending {
		return
	}

	r.store.Update(task.ID, func(t *Task) {
		t.Status = TaskStatusInProgress
		if t.Owner == "" {
			t.Owner = "task-runner"
		}
		if t.Metadata == nil {
			t.Metadata = map[string]any{}
		}
		t.Metadata["execStartedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	})
	r.progress(task, EventStarted)
	r.bump("executed")

	execCtx := ctx
	cancel := func() {}
	if r.opts.DefaultTimeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, r.opts.DefaultTimeout)
	}
	out, err := r.opts.Execute(execCtx, task)
	cancel()

	if err == nil {
		r.store.Update(task.ID, func(t *Task) {
			t.Status = TaskStatusCompleted
			if t.Checkpoint == nil {
				t.Checkpoint = map[string]any{}
			}
			t.Checkpoint["result"] = out
			if t.Metadata == nil {
				t.Metadata = map[string]any{}
			}
			t.Metadata["execFinishedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
		})
		r.progress(task, EventCompleted)
		r.bump("completed")
		return
	}

	// The run itself was cancelled/stopped (not a per-task timeout): park the
	// task as cancelled rather than consuming retry budget for an aborted run.
	if ctx.Err() != nil {
		if ok, _ := r.store.Cancel(task.ID, "run stopped"); ok {
			r.bump("cancelled")
		}
		r.progress(task, EventCancelled)
		return
	}

	requeued, err := r.store.MarkFailed(task.ID, err.Error())
	if err != nil {
		r.progress(task, EventFailed)
		r.bump("failed")
		return
	}
	if requeued {
		r.progress(task, EventRetrying)
		return
	}
	if r.tryReplan(ctx, task) {
		r.progress(task, EventReplanned)
		return
	}
	r.progress(task, EventFailed)
	r.bump("failed")
}

// tryReplan requeues a failed task with a fresh budget when the host's
// OnReplan hook approves and the per-task replan cap is not exhausted.
func (r *TaskRunner) tryReplan(ctx context.Context, task *Task) bool {
	if r.opts.OnReplan == nil {
		return false
	}
	r.mu.Lock()
	n := r.replansByTask[task.ID]
	r.mu.Unlock()
	if n >= r.opts.MaxReplans {
		return false
	}
	ok, err := r.opts.OnReplan(ctx, task)
	if err != nil || !ok {
		return false
	}
	if _, err := r.store.Requeue(task.ID); err != nil {
		return false
	}
	r.mu.Lock()
	r.replansByTask[task.ID]++
	r.replanned++
	r.mu.Unlock()
	return true
}

// dueTasks filters ready tasks to those whose retry backoff has elapsed. A task
// with no pending backoff tick is immediately due.
func (r *TaskRunner) dueTasks(tasks []*Task, now time.Time) []*Task {
	var due []*Task
	for _, t := range tasks {
		tick := metaInt(t.Metadata, "retryBackoffTick")
		if tick <= 0 {
			due = append(due, t)
			continue
		}
		if !now.Before(t.UpdatedAt.Add(r.backoff(tick))) {
			due = append(due, t)
		}
	}
	return due
}

// quiescent reports whether no ready work exists and nothing is in flight.
// Tasks blocked behind failed/skipped/cancelled dependencies are not "active":
// they never become ready, so they must not keep the run alive forever.
func (r *TaskRunner) quiescent() bool {
	if len(r.store.GetReadyWork()) > 0 {
		return false
	}
	for _, t := range r.store.List() {
		switch t.Status {
		case TaskStatusInProgress, TaskStatusReviewing:
			return false
		}
	}
	return true
}

// cancelActive parks all in-flight tasks as cancelled with the given reason.
func (r *TaskRunner) cancelActive(reason string) {
	for _, t := range r.store.List() {
		if t.Status != TaskStatusInProgress && t.Status != TaskStatusReviewing {
			continue
		}
		if ok, _ := r.store.Cancel(t.ID, reason); ok {
			r.bump("cancelled")
		}
	}
}

func (r *TaskRunner) terminalCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.completed + r.failed + r.skipped + r.cancelled
}

func (r *TaskRunner) bump(kind string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch kind {
	case "executed":
		r.executed++
	case "completed":
		r.completed++
	case "failed":
		r.failed++
	case "skipped":
		r.skipped++
	case "cancelled":
		r.cancelled++
	case "replanned":
		r.replanned++
	}
}

func (r *TaskRunner) progress(task *Task, event TaskRunnerEvent) {
	if r.opts.OnProgress != nil {
		r.opts.OnProgress(task, event)
	}
}

// Start launches Run in a background goroutine. Stop cancels it; Wait blocks
// until it finishes.
func (r *TaskRunner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.finished = false
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.doneCh = make(chan struct{})
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.finished = true
			r.mu.Unlock()
			close(r.doneCh)
		}()
		err := r.Run(runCtx)
		if err != nil {
			r.mu.Lock()
			r.runErr = err
			r.mu.Unlock()
		}
	}()
}

// RunErr returns the error from the background Run invocation, or nil if the
// runner has not finished or Run completed successfully.
func (r *TaskRunner) RunErr() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runErr
}

// Stop cancels a background run. Idempotent; safe when never started.
func (r *TaskRunner) Stop() {
	r.mu.Lock()
	c := r.cancel
	r.mu.Unlock()
	if c != nil {
		c()
	}
}

// Wait blocks until the background run finishes or ctx is done.
func (r *TaskRunner) Wait(ctx context.Context) error {
	r.mu.Lock()
	done := r.doneCh
	r.mu.Unlock()
	if done == nil {
		return fmt.Errorf("task runner: not started")
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status returns a point-in-time snapshot of runner activity.
func (r *TaskRunner) Status() TaskRunnerStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return TaskRunnerStats{
		Running:   r.started && !r.finished,
		Executed:  r.executed,
		Completed: r.completed,
		Failed:    r.failed,
		Skipped:   r.skipped,
		Cancelled: r.cancelled,
		Replanned: r.replanned,
	}
}

// metaInt reads an integer metadata value that may have been round-tripped
// through JSON (float64) or set directly in memory (int).
func metaInt(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
