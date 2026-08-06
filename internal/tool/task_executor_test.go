package tool

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTaskExecutor records the tasks it runs and fails those on the fail list.
type fakeTaskExecutor struct {
	mu     sync.Mutex
	ran    []string
	fail   map[string]error
	delay  time.Duration
	output map[string]string
}

func newFakeExecutor(fail map[string]error) *fakeTaskExecutor {
	return &fakeTaskExecutor{
		fail:   fail,
		output: map[string]string{},
	}
}

func (f *fakeTaskExecutor) execute(ctx context.Context, t *Task) (string, error) {
	f.mu.Lock()
	f.ran = append(f.ran, t.ID)
	err := f.fail[t.ID]
	out := f.output[t.ID]
	f.mu.Unlock()
	if ctx != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "done:" + t.ID
	}
	return out, nil
}

func newStoreWith(tasks ...func(*Task)) *TaskStore {
	s := &TaskStore{tasks: make(map[string]*Task)}
	for _, setup := range tasks {
		t := s.Create("subject", "description", "", nil)
		setup(t)
	}
	return s
}

func TestTaskRunnerCompletesReadyWork(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	a := s.Create("a", "do a", "", nil)
	b := s.Create("b", "do b", "", nil)

	exec := newFakeExecutor(nil)
	r := NewTaskRunner(TaskRunnerOptions{
		Store:        s,
		Execute:      exec.execute,
		PollInterval: time.Millisecond,
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(a.ID)
	if got.Status != TaskStatusCompleted {
		t.Fatalf("task a status = %q, want completed", got.Status)
	}
	gotB, _ := s.Get(b.ID)
	if gotB.Status != TaskStatusCompleted {
		t.Fatalf("task b status = %q, want completed", gotB.Status)
	}
	st := r.Status()
	if st.Completed != 2 || st.Executed != 2 {
		t.Fatalf("unexpected stats: %+v", st)
	}
	if got.Checkpoint["result"] != "done:task_1" {
		t.Fatalf("expected result checkpoint, got %+v", got.Checkpoint)
	}
}

func TestTaskRunnerRespectsDependencies(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	a := s.Create("a", "do a", "", nil)
	b := s.Create("b", "do b", "", nil)
	s.Update(b.ID, func(t *Task) {
		t.Dependencies = []TaskDependency{{TargetID: a.ID, Type: "blocks"}}
	})

	exec := newFakeExecutor(nil)
	r := NewTaskRunner(TaskRunnerOptions{
		Store:        s,
		Execute:      exec.execute,
		PollInterval: time.Millisecond,
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	exec.mu.Lock()
	defer exec.mu.Unlock()
	if len(exec.ran) != 2 {
		t.Fatalf("expected 2 executions, got %v", exec.ran)
	}
	if exec.ran[0] != a.ID || exec.ran[1] != b.ID {
		t.Fatalf("dependency order violated: %v", exec.ran)
	}
}

func TestTaskRunnerRetriesWithinBudget(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	task := s.Create("flaky", "do flaky", "", nil)
	s.Update(task.ID, func(t *Task) { t.MaxAttempts = 3 })

	// Fail the first two attempts, succeed on the third.
	attempts := map[string]int{}
	var mu sync.Mutex
	exec := func(_ context.Context, t *Task) (string, error) {
		mu.Lock()
		attempts[t.ID]++
		n := attempts[t.ID]
		mu.Unlock()
		if n < 3 {
			return "", fmt.Errorf("transient failure %d", n)
		}
		return "ok", nil
	}

	r := NewTaskRunner(TaskRunnerOptions{
		Store:        s,
		Execute:      exec,
		Backoff:      func(int) time.Duration { return time.Millisecond },
		PollInterval: time.Millisecond,
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(task.ID)
	if got.Status != TaskStatusCompleted {
		t.Fatalf("expected completed after retries, got %q (lastError=%q)", got.Status, got.LastError)
	}
	if got.Attempts != 2 {
		t.Fatalf("expected 2 failed attempts recorded, got %d", got.Attempts)
	}
}

func TestTaskRunnerParksFailedAfterBudget(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	task := s.Create("doomed", "do doomed", "", nil)
	s.Update(task.ID, func(t *Task) { t.MaxAttempts = 2 })

	exec := func(_ context.Context, t *Task) (string, error) {
		return "", fmt.Errorf("always fails")
	}

	r := NewTaskRunner(TaskRunnerOptions{
		Store:        s,
		Execute:      exec,
		Backoff:      func(int) time.Duration { return time.Millisecond },
		PollInterval: time.Millisecond,
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(task.ID)
	if got.Status != TaskStatusFailed {
		t.Fatalf("expected failed after budget, got %q", got.Status)
	}
	if got.Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", got.Attempts)
	}
	st := r.Status()
	if st.Failed != 1 {
		t.Fatalf("expected 1 failed in stats, got %+v", st)
	}
}

func TestTaskRunnerReplansFailedTask(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	task := s.Create("replan", "do replan", "", nil)
	s.Update(task.ID, func(t *Task) { t.MaxAttempts = 1 })

	// Fail the first round (budget 1), then replan requeues it; succeed after.
	round := 0
	exec := func(_ context.Context, t *Task) (string, error) {
		if round == 0 {
			round = 1
			return "", fmt.Errorf("first round fails")
		}
		return "ok", nil
	}
	replans := 0
	r := NewTaskRunner(TaskRunnerOptions{
		Store:        s,
		Execute:      exec,
		Backoff:      func(int) time.Duration { return time.Millisecond },
		PollInterval: time.Millisecond,
		OnReplan: func(_ context.Context, t *Task) (bool, error) {
			replans++
			return true, nil
		},
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(task.ID)
	if got.Status != TaskStatusCompleted {
		t.Fatalf("expected completed after replan, got %q", got.Status)
	}
	if replans != 1 {
		t.Fatalf("expected 1 replan, got %d", replans)
	}
	st := r.Status()
	if st.Replanned != 1 {
		t.Fatalf("expected 1 replanned in stats, got %+v", st)
	}
}

func TestTaskRunnerWatchdogStops(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	for i := 0; i < 10; i++ {
		s.Create(fmt.Sprintf("t%d", i), "desc", "", nil)
	}

	exec := newFakeExecutor(nil)
	r := NewTaskRunner(TaskRunnerOptions{
		Store:         s,
		Execute:       exec.execute,
		PollInterval:  time.Millisecond,
		MaxTotalTasks: 3,
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	exec.mu.Lock()
	n := len(exec.ran)
	exec.mu.Unlock()
	if n != 3 {
		t.Fatalf("watchdog should stop after 3 tasks, ran %d", n)
	}
	st := r.Status()
	if st.Completed != 3 {
		t.Fatalf("expected 3 completed, got %+v", st)
	}
	// Remaining tasks stay pending.
	remaining := 0
	for _, t := range s.List() {
		if t.Status == TaskStatusPending {
			remaining++
		}
	}
	if remaining != 7 {
		t.Fatalf("expected 7 pending remaining, got %d", remaining)
	}
}

func TestTaskRunnerCancelsInFlightOnStop(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	task := s.Create("slow", "slow task", "", nil)

	started := make(chan struct{})
	exec := func(ctx context.Context, t *Task) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	}

	r := NewTaskRunner(TaskRunnerOptions{
		Store:          s,
		Execute:        exec,
		PollInterval:   time.Millisecond,
		DefaultTimeout: 5 * time.Second,
	})
	ctx := context.Background()
	r.Start(ctx)
	<-started
	r.Stop()
	if err := r.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, _ := s.Get(task.ID)
	if got.Status != TaskStatusCancelled {
		t.Fatalf("expected cancelled on stop, got %q", got.Status)
	}
}

func TestTaskRunnerQuiescesBehindFailedBlocker(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	a := s.Create("a", "do a", "", nil)
	b := s.Create("b", "depends on a", "", nil)
	s.Update(b.ID, func(t *Task) { t.Dependencies = []TaskDependency{{TargetID: a.ID, Type: "blocks"}} })
	s.Update(a.ID, func(t *Task) { t.MaxAttempts = 1 })
	// Mark a failed so b can never become ready.
	s.MarkFailed(a.ID, "cannot do a")

	exec := newFakeExecutor(nil)
	r := NewTaskRunner(TaskRunnerOptions{
		Store:        s,
		Execute:      exec.execute,
		PollInterval: time.Millisecond,
	})
	// Must terminate rather than spin forever behind the failed blocker.
	done := make(chan error, 1)
	go func() { done <- r.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runner spun forever behind a failed blocker")
	}
}

func TestTaskRunnerProgressEvents(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	s.Create("a", "do a", "", nil)

	var events []string
	var mu sync.Mutex
	exec := newFakeExecutor(nil)
	r := NewTaskRunner(TaskRunnerOptions{
		Store:        s,
		Execute:      exec.execute,
		PollInterval: time.Millisecond,
		OnProgress: func(t *Task, e TaskRunnerEvent) {
			mu.Lock()
			events = append(events, string(e))
			mu.Unlock()
		},
	})
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(events, ",")
	if !strings.Contains(joined, string(EventStarted)) || !strings.Contains(joined, string(EventCompleted)) {
		t.Fatalf("expected started+completed events, got %q", joined)
	}
}

func TestSkipAndCancel(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	task := s.Create("a", "do a", "", nil)

	if ok, _ := s.Skip(task.ID, "not needed"); !ok {
		t.Fatal("skip should succeed")
	}
	got, _ := s.Get(task.ID)
	if got.Status != TaskStatusSkipped || got.Metadata["skipReason"] != "not needed" {
		t.Fatalf("unexpected after skip: %+v", got)
	}
	// Skipping a completed task is a no-op.
	s.Update(task.ID, func(t *Task) { t.Status = TaskStatusPending })
	if ok, _ := s.Skip(task.ID, "x"); !ok {
		t.Fatal("re-skip of pending should succeed")
	}
	s.Update(task.ID, func(t *Task) { t.Status = TaskStatusCompleted })
	if ok, _ := s.Cancel(task.ID, "too late"); ok {
		t.Fatal("cancelling a completed task should be a no-op")
	}

	c := s.Create("c", "do c", "", nil)
	if ok, _ := s.Cancel(c.ID, "aborted"); !ok {
		t.Fatal("cancel should succeed")
	}
	gotC, _ := s.Get(c.ID)
	if gotC.Status != TaskStatusCancelled || gotC.Metadata["cancelReason"] != "aborted" {
		t.Fatalf("unexpected after cancel: %+v", gotC)
	}
}

func TestTaskRunnerMissingExecute(t *testing.T) {
	s := &TaskStore{tasks: make(map[string]*Task)}
	r := NewTaskRunner(TaskRunnerOptions{Store: s})
	if err := r.Run(context.Background()); err == nil {
		t.Fatal("expected error when execute is missing")
	}
}
