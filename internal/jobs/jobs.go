// Package jobs is a Go-native port of DSH's background-job service
// (`jobs/jobs`, dsh-v0.1.0-rc.7). It owns the contract for job ids,
// session-scoped access, lifecycle state, completion listeners, and owner
// cleanup while producers retain their execution resources.
//
// A producer declares work through [Start]; the runtime preflights access and
// cleanup before invoking the producer's Run, then owns identity and
// lifecycle state. The producer owns execution resources and signals
// termination through the [Hooks] it returns.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Kind is an opaque producer-defined job kind (the id prefix: `bash`,
// `subagent`, …).
type Kind string

// Status is the job lifecycle state: `running`, optionally `stopping`, then
// exactly one terminal status. Producer-specific facts belong in
// [Snapshot.Detail].
type Status string

const (
	StatusRunning   Status = "running"
	StatusStopping  Status = "stopping"
	StatusCompleted Status = "completed"
	StatusKilled    Status = "killed"
	StatusFailed    Status = "failed"
)

// Outcome is the terminal result supplied by a producer through [Hooks.Done].
type Outcome struct {
	// How the job ended: finished (`completed`), cancelled (`killed`), or
	// broke (`failed`).
	Status Status
	// Kind-specific detail rendered into status lines ('exit code: 3',
	// 'max-tokens').
	Detail string
	// Final output for jobs without streamed reads; stream jobs leave it unset.
	Output string
}

// Start is the producer declaration passed to [Registry.Start].
type Start struct {
	// Producer kind — also the id prefix (`bash`, `subagent`, …).
	Kind Kind
	// One-line model-facing label (the command; the delegation description).
	Label string
	// Optional UTF-8 byte cap for each complete model-facing completion
	// notice or output read.
	OutputLimitBytes int
	// Owner session id used for authorization and correlation; empty for an
	// unowned job, open to any caller until service disposal.
	OwnerSession string
	// Start the work after preflight and synchronously return its hooks.
	// Called once; a panic leaves nothing registered.
	Run func() Hooks
}

// Snapshot is a read-only projection of one job, safe to hand to listeners
// and tools — a fresh object per call, never live registry state.
type Snapshot struct {
	// The registry-issued id (`<kind>-N`).
	ID ID
	// The producer kind the job was registered with.
	Kind Kind
	// The producer-supplied one-line label.
	Label string
	// Producer-owned cap for complete model-facing notices and output reads.
	OutputLimitBytes int
	// Owner session id used for authorization and correlation; empty for
	// unowned jobs.
	OwnerSession string
	// Current lifecycle state.
	Status Status
	// Kind-specific status detail, present once the producer supplied one.
	Detail string
	// Time the job was registered.
	StartedAt time.Time
	// Time the job settled; zero while running/stopping.
	FinishedAt time.Time
	// True when a kill, read, wait, or teardown cancel has reported or
	// committed to report the terminal state. Completion reporters suppress
	// redundant notices when set.
	Reported bool
}

// Read is the output and post-read state returned by [Registry.Read].
type Read struct {
	// Stream kinds: the consuming delta since the previous read. Final-output
	// kinds: empty while live, the terminal Outcome.Output (or empty) once
	// settled — idempotent, never consumed.
	Text string
	// The job's state at read time.
	Snapshot Snapshot
}

// DoneListener is a completion callback. The owner is the exact owner
// supplied at Start, or "" for an unowned job.
type DoneListener func(snapshot Snapshot, owner string)

// ChangedListener observes a change to what one owner's [Registry.List] would
// return. It is owner-granular rather than job-granular because the change
// may be a removal. An empty owner means an unowned job changed, so every
// caller's visible set changed with it.
type ChangedListener func(owner string)

// ID is the registry-issued job id (`<kind>-N`).
type ID string

// job is one registered job. Guarded by the registry mutex; the producer's
// Done callback runs on the producer's goroutine and takes the mutex.
type job struct {
	id       ID
	kind     Kind
	label    string
	limit    int
	owner    string
	status   Status
	detail   string
	started  time.Time
	finished time.Time
	reported bool

	done chan struct{}

	// hooks installed synchronously in Start before it returns; never nil
	// afterwards.
	hooks Hooks
	// pendingCancel records a Kill that arrived before Run returned hooks so
	// the cancel is delivered once hooks exist.
	pendingCancel string
	// lastRead is the output length consumed by the previous Read.
	lastRead int
	// settled records that Done delivered its terminal outcome.
	settled bool
}

// Registry is a process-local job registry (DSH `jobs/jobs-local` parity).
type Registry struct {
	mu    sync.Mutex
	next  int
	jobs  map[ID]*job
	done  map[ID][]DoneListener
	chg   []ChangedListener
	order []ID
	// shuttingDown disallows new Starts during Close/ReleaseOwner teardown.
	shuttingDown bool
}

// ErrNotFound is returned when a job id is unknown or already removed.
var ErrNotFound = errors.New("jobs: no such job")

// ErrShuttingDown is returned by Start while the registry is closing.
var ErrShuttingDown = errors.New("jobs: registry is shutting down")

// NewRegistry creates an empty process-local registry.
func NewRegistry() *Registry {
	return &Registry{
		jobs: make(map[ID]*job),
		done: make(map[ID][]DoneListener),
	}
}

// Hooks are the handles through which the runtime controls and observes one
// producer's work, returned synchronously from [Start.Run].
type Hooks struct {
	// Cancel requests termination. It must be synchronous, idempotent, and
	// eventually send on Done; the optional reason is forwarded verbatim.
	Cancel func(reason string)
	// Done is the producer's completion promise (DSH's `done`). The producer
	// sends exactly one terminal outcome when it releases its resources; the
	// registry drains it and settles registry state. A buffered channel of
	// size 1 is recommended.
	Done chan Outcome
	// ReadOutput returns the job's complete current output (used by stream
	// jobs to compute the delta since the last read).
	ReadOutput func() string
}

// Start registers a new job and runs the producer synchronously to obtain its
// hooks. The producer owns execution resources and signals termination by
// sending on [Hooks.Done] (DSH's `done` promise); the registry owns identity
// and lifecycle state. A panic in Run leaves nothing registered and propagates
// as an error, matching DSH's "a throw leaves nothing registered" contract.
func (r *Registry) Start(s Start) (id ID, err error) {
	r.mu.Lock()
	if r.shuttingDown {
		r.mu.Unlock()
		return "", ErrShuttingDown
	}
	r.next++
	id = ID(fmt.Sprintf("%s-%d", s.Kind, r.next))
	j := &job{
		id:      id,
		kind:    s.Kind,
		label:   s.Label,
		limit:   s.OutputLimitBytes,
		owner:   s.OwnerSession,
		status:  StatusRunning,
		started: time.Now(),
		done:    make(chan struct{}),
	}
	r.jobs[id] = j
	r.order = append(r.order, id)
	r.mu.Unlock()

	// Run synchronously so hooks are installed before Start returns; the
	// producer starts its async work and returns immediately.
	var hooks Hooks
	func() {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("jobs: producer %s panicked: %v", id, p)
			}
		}()
		hooks = s.Run()
	}()
	if err != nil {
		r.mu.Lock()
		delete(r.jobs, id)
		r.mu.Unlock()
		return "", err
	}

	var pending string
	r.mu.Lock()
	j.hooks = hooks
	if j.status == StatusStopping && j.pendingCancel != "" {
		pending = j.pendingCancel
	}
	r.mu.Unlock()
	if pending != "" && hooks.Cancel != nil {
		hooks.Cancel(pending)
	}

	// Drain the producer's completion promise (DSH: `void hooks.done.then(...)`)
	// and settle registry state on the producer's goroutine.
	go func() {
		o := <-hooks.Done
		r.fireDone(id, o)
	}()
	return id, nil
}

// Snapshot returns a fresh projection of the job.
func (r *Registry) Snapshot(id ID) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return Snapshot{}, ErrNotFound
	}
	return r.snapshotLocked(j), nil
}

func (r *Registry) snapshotLocked(j *job) Snapshot {
	s := Snapshot{
		ID:               j.id,
		Kind:             j.kind,
		Label:            j.label,
		OutputLimitBytes: j.limit,
		OwnerSession:     j.owner,
		Status:           j.status,
		Detail:           j.detail,
		StartedAt:        j.started,
		FinishedAt:       j.finished,
		Reported:         j.reported,
	}
	return s
}

// List returns snapshots of every job visible to owner: the owner's own jobs
// plus every unowned job (unowned jobs are open to any caller, matching DSH).
// An empty owner returns only unowned jobs.
func (r *Registry) List(owner string) []Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Snapshot
	for _, id := range r.order {
		j := r.jobs[id]
		if j == nil {
			continue
		}
		if j.owner != "" && (owner == "" || j.owner != owner) {
			continue
		}
		out = append(out, r.snapshotLocked(j))
	}
	return out
}

// Read returns the consuming output delta and the job's state. Stream kinds
// return the delta since the previous read; final-output kinds return the
// terminal Outcome.Output once settled (idempotent, never consumed).
func (r *Registry) Read(id ID) (Read, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return Read{}, ErrNotFound
	}
	snap := r.snapshotLocked(j)
	out := Read{Snapshot: snap}
	switch {
	case snap.Status == StatusCompleted || snap.Status == StatusKilled || snap.Status == StatusFailed:
		// Final-output kinds: terminal output, idempotent.
		if j.hooks.ReadOutput != nil {
			out.Text = j.hooks.ReadOutput()
		}
	case j.hooks.ReadOutput != nil:
		// Stream kinds: delta since the previous read.
		full := j.hooks.ReadOutput()
		if len(full) > j.lastRead {
			out.Text = full[j.lastRead:]
			j.lastRead = len(full)
		}
	}
	// Reading a terminal job reports it, suppressing redundant notices.
	if (snap.Status == StatusCompleted || snap.Status == StatusKilled || snap.Status == StatusFailed) && !j.reported {
		j.reported = true
		snap.Reported = true
		out.Snapshot = snap
	}
	return out, nil
}

// Wait blocks until the job settles or ctx is done, then returns its
// snapshot. The settle is reported so redundant notices are suppressed.
func (r *Registry) Wait(ctx context.Context, id ID) (Snapshot, error) {
	for {
		r.mu.Lock()
		j, ok := r.jobs[id]
		if !ok {
			r.mu.Unlock()
			return Snapshot{}, ErrNotFound
		}
		if j.status == StatusCompleted || j.status == StatusKilled || j.status == StatusFailed {
			if !j.reported {
				j.reported = true
			}
			snap := r.snapshotLocked(j)
			r.mu.Unlock()
			return snap, nil
		}
		done := j.done
		r.mu.Unlock()

		select {
		case <-ctx.Done():
			return Snapshot{}, ctx.Err()
		case <-done:
		}
	}
}

// Kill requests termination of a running job. It is idempotent: a second kill
// of an already-terminal job is a no-op. If the producer has not yet returned
// its hooks, the cancel reason is stashed and delivered once hooks exist.
func (r *Registry) Kill(id ID, reason string) error {
	r.mu.Lock()
	j, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return ErrNotFound
	}
	terminal := j.status == StatusCompleted || j.status == StatusKilled || j.status == StatusFailed
	if j.status == StatusRunning {
		j.status = StatusStopping
	}
	hooks := j.hooks
	if hooks.Cancel == nil && !terminal && j.pendingCancel == "" {
		j.pendingCancel = reason
	}
	if terminal && !j.reported {
		j.reported = true
	}
	r.mu.Unlock()

	if hooks.Cancel != nil && !terminal {
		hooks.Cancel(reason)
	}
	return nil
}

// OnDone registers a completion listener for one job. The listener is invoked
// exactly once, on the goroutine that settles the job; it is removed after
// firing. If the job is already terminal, the listener fires immediately.
func (r *Registry) OnDone(id ID, fn DoneListener) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if j.status == StatusCompleted || j.status == StatusKilled || j.status == StatusFailed {
		go fn(r.snapshotLocked(j), j.owner)
		return nil
	}
	r.done[id] = append(r.done[id], fn)
	return nil
}

// OnChanged registers an owner-granular change listener and returns a
// disposer. It fires when any job for owner settles or is removed; an empty
// owner means unowned jobs changed.
func (r *Registry) OnChanged(fn ChangedListener) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.chg = append(r.chg, fn)
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, f := range r.chg {
			if &f == &fn {
				r.chg = append(r.chg[:i], r.chg[i+1:]...)
				return
			}
		}
	}
}

// fireDone settles the job from a producer Done call. It must run on the
// producer goroutine; the registry transitions state and notifies listeners.
func (r *Registry) fireDone(id ID, o Outcome) {
	r.mu.Lock()
	j, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	if j.settled {
		r.mu.Unlock()
		return
	}
	j.settled = true
	j.status = o.Status
	j.detail = o.Detail
	j.finished = time.Now()
	close(j.done)
	snap := r.snapshotLocked(j)
	listeners := append([]DoneListener(nil), r.done[id]...)
	delete(r.done, id)
	owners := map[string]bool{j.owner: true}
	r.mu.Unlock()

	for _, fn := range listeners {
		fn(snap, j.owner)
	}
	r.notifyChanged(owners)
}

// notifyChanged fires owner-granular change listeners.
func (r *Registry) notifyChanged(owners map[string]bool) {
	r.mu.Lock()
	listeners := append([]ChangedListener(nil), r.chg...)
	r.mu.Unlock()
	for _, fn := range listeners {
		for owner := range owners {
			fn(owner)
		}
		if len(owners) == 0 {
			fn("")
		}
	}
}

// ReleaseOwner cancels and awaits every job owned by owner, then removes
// them. It is the port of DSH's agent-disposal cleanup ("cancels and awaits
// the job").
func (r *Registry) ReleaseOwner(owner string) error {
	r.mu.Lock()
	var owned []*job
	for _, id := range r.order {
		j := r.jobs[id]
		if j != nil && j.owner == owner {
			owned = append(owned, j)
			if j.status == StatusRunning {
				j.status = StatusStopping
			}
		}
	}
	hooks := make(map[*job]Hooks)
	for _, j := range owned {
		hooks[j] = j.hooks
	}
	r.mu.Unlock()

	for j, h := range hooks {
		if h.Cancel != nil && (j.status == StatusStopping || j.status == StatusRunning) {
			h.Cancel("owner released")
		}
	}

	// Await settlement for a bounded grace period; the producer must settle
	// itself after Cancel.
	deadline := time.After(5 * time.Second)
	for _, j := range owned {
		select {
		case <-j.done:
		case <-deadline:
			return errors.New("jobs: owner release timed out awaiting job settlement")
		}
	}

	r.mu.Lock()
	changed := make(map[string]bool)
	for _, j := range owned {
		if _, ok := r.jobs[j.id]; ok {
			delete(r.jobs, j.id)
			delete(r.done, j.id)
			changed[j.owner] = true
		}
	}
	r.mu.Unlock()
	r.notifyChanged(changed)
	return nil
}

// Close disallows new starts, cancels all running jobs, awaits settlement,
// and clears the registry (DSH service disposal semantics).
func (r *Registry) Close() error {
	r.mu.Lock()
	r.shuttingDown = true
	var all []*job
	for _, id := range r.order {
		if j := r.jobs[id]; j != nil {
			if j.status == StatusRunning {
				j.status = StatusStopping
			}
			all = append(all, j)
		}
	}
	hooks := make(map[*job]Hooks)
	for _, j := range all {
		hooks[j] = j.hooks
	}
	r.mu.Unlock()

	for j, h := range hooks {
		if h.Cancel != nil && (j.status == StatusStopping || j.status == StatusRunning) {
			h.Cancel("registry closed")
		}
	}

	deadline := time.After(5 * time.Second)
	for _, j := range all {
		select {
		case <-j.done:
		case <-deadline:
			return errors.New("jobs: close timed out awaiting job settlement")
		}
	}

	r.mu.Lock()
	r.jobs = make(map[ID]*job)
	r.done = make(map[ID][]DoneListener)
	r.chg = nil
	r.order = nil
	r.mu.Unlock()
	return nil
}
