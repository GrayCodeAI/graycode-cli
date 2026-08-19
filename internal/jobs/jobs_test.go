package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// controlledHooks lets a test drive a producer's lifecycle deterministically.
type controlledHooks struct {
	cancel   chan string
	done     chan Outcome
	outputMu sync.Mutex
	current  string
	// onCancel, when set, settles the job through the registry when Cancel is
	// invoked — the contract a real producer must honour.
	onCancel func(reason string)
}

// setOutput replaces the streamed output (guarded, producer-side).
func (ch *controlledHooks) setOutput(s string) {
	ch.outputMu.Lock()
	defer ch.outputMu.Unlock()
	ch.current = s
}

// producer returns a Start whose Run hands back controllable hooks. The test
// settles the job by calling r.fireDone(id, outcome) directly, exactly as the
// producer's Done hook would.
func producer(owner string) (Start, *controlledHooks) {
	ch := &controlledHooks{
		cancel: make(chan string, 1),
		done:   make(chan Outcome, 1),
	}
	s := Start{
		Kind:         Kind("bash"),
		Label:        "echo hi",
		OwnerSession: owner,
		Run: func() Hooks {
			return Hooks{
				Cancel: func(reason string) {
					ch.cancel <- reason
					if ch.onCancel != nil {
						ch.onCancel(reason)
					}
				},
				Done: ch.done,
				ReadOutput: func() string {
					ch.outputMu.Lock()
					defer ch.outputMu.Unlock()
					return ch.current
				},
			}
		},
	}
	return s, ch
}

func TestStartAndDoneLifecycle(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	s, ch := producer("sess-1")

	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(id) != "bash-1" {
		t.Fatalf("id = %q, want bash-1", id)
	}

	snap, err := r.Snapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != StatusRunning || snap.OwnerSession != "sess-1" || snap.Label != "echo hi" {
		t.Fatalf("snapshot = %+v", snap)
	}

	doneCh := make(chan Snapshot, 1)
	if err := r.OnDone(id, func(sn Snapshot, owner string) {
		if owner != "sess-1" {
			t.Errorf("listener owner = %q, want sess-1", owner)
		}
		doneCh <- sn
	}); err != nil {
		t.Fatal(err)
	}

	// Settle through the producer's completion promise (the registry's drain
	// goroutine transitions state).
	ch.done <- Outcome{Status: StatusCompleted, Detail: "exit code: 0"}
	select {
	case sn := <-doneCh:
		if sn.Status != StatusCompleted || sn.Detail != "exit code: 0" {
			t.Fatalf("done snapshot = %+v", sn)
		}
		if sn.FinishedAt.IsZero() {
			t.Fatal("finishedAt not set")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("done listener never fired")
	}

	// A second settle is a no-op (settled guard).
	ch.done <- Outcome{Status: StatusFailed, Detail: "late"}
	if snap, _ := r.Snapshot(id); snap.Status != StatusCompleted {
		t.Fatalf("status changed after duplicate settle: %+v", snap)
	}
}

func TestKillTransitionsToTerminal(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	s, ch := producer("")
	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Kill(id, "user cancelled"); err != nil {
		t.Fatal(err)
	}
	select {
	case reason := <-ch.cancel:
		if reason != "user cancelled" {
			t.Fatalf("cancel reason = %q", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel hook never called")
	}

	snap, _ := r.Snapshot(id)
	if snap.Status != StatusStopping {
		t.Fatalf("status = %q, want stopping", snap.Status)
	}

	r.fireDone(id, Outcome{Status: StatusKilled, Detail: "killed by user"})
	snap, err = r.Wait(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != StatusKilled || snap.Detail != "killed by user" {
		t.Fatalf("snapshot = %+v", snap)
	}
	// Wait reports the terminal state.
	if !snap.Reported {
		t.Fatal("Wait should report the terminal state")
	}
}

func TestKillIdempotentAfterTerminal(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	s, ch := producer("")
	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}
	r.fireDone(id, Outcome{Status: StatusCompleted})
	_, _ = r.Wait(context.Background(), id)

	if err := r.Kill(id, "again"); err != nil {
		t.Fatalf("second kill errored: %v", err)
	}
	select {
	case <-ch.cancel:
		t.Fatal("cancel hook called after terminal")
	default:
	}
}

func TestWaitBlocksUntilSettled(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	s, _ := producer("")
	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		time.Sleep(20 * time.Millisecond)
		r.fireDone(id, Outcome{Status: StatusFailed, Detail: "boom"})
	}()

	snap, err := r.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != StatusFailed || snap.Detail != "boom" {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestWaitRespectsContextCancel(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	s, ch := producer("")
	var id ID
	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}
	ch.onCancel = func(string) { r.fireDone(id, Outcome{Status: StatusKilled, Detail: "cancelled"}) }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := r.Wait(ctx, id); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait = %v, want DeadlineExceeded", err)
	}
}

func TestReadStreamDeltaAndFinalOutput(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	// Stream job: output grows.
	s, ch := producer("")
	s.Label = "stream"
	var id ID
	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}
	ch.onCancel = func(string) { r.fireDone(id, Outcome{Status: StatusKilled, Detail: "cancelled"}) }

	ch.setOutput("hello ")
	rd, err := r.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	if rd.Text != "hello " {
		t.Fatalf("first read = %q, want delta", rd.Text)
	}

	ch.setOutput("hello world")
	rd, _ = r.Read(id)
	if rd.Text != "world" {
		t.Fatalf("second read = %q, want only the delta", rd.Text)
	}
}

func TestReadFinalOutputIdempotent(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	s, ch := producer("")
	s.Label = "final"
	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}
	ch.setOutput("final output")
	r.fireDone(id, Outcome{Status: StatusCompleted, Output: "final output"})

	rd, err := r.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	if rd.Text != "final output" {
		t.Fatalf("read = %q", rd.Text)
	}
	rd2, _ := r.Read(id)
	if rd2.Text != "final output" {
		t.Fatalf("second read = %q, want idempotent", rd2.Text)
	}
	if !rd.Snapshot.Reported {
		t.Fatal("first read should set reported")
	}
	if rd2.Snapshot.Reported != rd.Snapshot.Reported {
		t.Fatal("reported must be stable across reads")
	}
}

func TestListOwnerScoping(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	for _, owner := range []string{"sess-a", "sess-b", ""} {
		s, ch := producer(owner)
		var id ID
		id, err := r.Start(s)
		if err != nil {
			t.Fatal(err)
		}
		ch.onCancel = func(string) { r.fireDone(id, Outcome{Status: StatusKilled, Detail: "cancelled"}) }
	}

	// A session sees its own jobs plus every unowned job.
	if got := len(r.List("sess-a")); got != 2 {
		t.Fatalf("List(sess-a) len = %d, want 2 (own + unowned)", got)
	}
	if got := len(r.List("sess-b")); got != 2 {
		t.Fatalf("List(sess-b) len = %d, want 2 (own + unowned)", got)
	}
	// Anonymous caller sees only unowned jobs.
	if got := len(r.List("")); got != 1 {
		t.Fatalf("List(unowned) len = %d, want 1", got)
	}
	// A stranger sees only unowned jobs.
	if got := len(r.List("sess-unknown")); got != 1 {
		t.Fatalf("List(unknown) len = %d, want 1 (unowned only)", got)
	}
}

func TestReleaseOwnerRemovesOnlyOwnerJobs(t *testing.T) {
	r := NewRegistry()
	defer r.Close()

	so, chO := producer("owner-1")
	su, chU := producer("")
	var idO, idU ID
	idO, err := r.Start(so)
	if err != nil {
		t.Fatal(err)
	}
	idU, err = r.Start(su)
	if err != nil {
		t.Fatal(err)
	}
	chO.onCancel = func(string) { r.fireDone(idO, Outcome{Status: StatusKilled, Detail: "cancelled"}) }
	chU.onCancel = func(string) { r.fireDone(idU, Outcome{Status: StatusKilled, Detail: "cancelled"}) }

	changed := make(chan string, 4)
	dispose := r.OnChanged(func(owner string) { changed <- owner })
	defer dispose()

	r.fireDone(idO, Outcome{Status: StatusCompleted})
	if err := r.ReleaseOwner("owner-1"); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Snapshot(idO); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner job still present: %v", err)
	}
	if _, err := r.Snapshot(idU); err != nil {
		t.Fatalf("unowned job removed: %v", err)
	}
	select {
	case owner := <-changed:
		if owner != "owner-1" {
			t.Fatalf("changed owner = %q", owner)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("change listener never fired")
	}
}

func TestCloseDisallowsNewStartsAndCleansUp(t *testing.T) {
	r := NewRegistry()
	var id ID
	id, err := r.Start(Start{Kind: Kind("bash"), Label: "x", Run: func() Hooks {
		// The producer settles when cancelled, as a real producer must.
		done := make(chan Outcome, 1)
		return Hooks{
			Cancel: func(string) { done <- Outcome{Status: StatusKilled, Detail: "cancelled"} },
			Done:   done,
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Snapshot(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("job survived close: %v", err)
	}
	if _, err := r.Start(Start{Kind: Kind("bash"), Label: "y", Run: func() Hooks {
		return Hooks{Done: make(chan Outcome, 1)}
	}}); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("Start after close = %v, want ErrShuttingDown", err)
	}
}

func TestUnknownJobErrors(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	if _, err := r.Snapshot("nope-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Snapshot = %v, want ErrNotFound", err)
	}
	if _, err := r.Read("nope-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Read = %v, want ErrNotFound", err)
	}
	if err := r.Kill("nope-1", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Kill = %v, want ErrNotFound", err)
	}
	if err := r.OnDone("nope-1", func(Snapshot, string) {}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OnDone = %v, want ErrNotFound", err)
	}
}

func TestOnDoneAlreadyTerminalFiresImmediately(t *testing.T) {
	r := NewRegistry()
	defer r.Close()
	s, _ := producer("")
	id, err := r.Start(s)
	if err != nil {
		t.Fatal(err)
	}
	r.fireDone(id, Outcome{Status: StatusCompleted})

	doneCh := make(chan struct{}, 1)
	if err := r.OnDone(id, func(Snapshot, string) { doneCh <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("already-terminal listener never fired")
	}
}
