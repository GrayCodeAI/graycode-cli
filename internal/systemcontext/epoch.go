package systemcontext

// Epoch ties a durable baseline, its snapshot, and the sequence bookkeeping
// together. An Epoch starts at a fresh baseline and advances atomically as
// changed context is admitted at safe provider-turn boundaries.
//
// This mirrors opencode's Context Epoch: the baseline remains the immutable
// provider-cache prefix for the span of the epoch, and only delta updates are
// admitted as chronological messages.
type Epoch struct {
	// Baseline is the exact, immutable system-prompt prefix for this epoch.
	Baseline string
	// Snapshot is the model-hidden comparison state that advances with updates.
	Snapshot *Snapshot
}

// NewEpoch constructs an epoch from an already-computed baseline and snapshot.
func NewEpoch(baseline string, snap *Snapshot) *Epoch {
	if snap == nil {
		snap = NewSnapshot()
	}
	return &Epoch{Baseline: baseline, Snapshot: snap}
}

// Prepare admits context changes at a safe provider-turn boundary.
//
// It returns nil when nothing changed, an update message when one or more
// sources changed (advancing the epoch's snapshot), or (true, _, _) when the
// baseline must be replaced.
func (e *Epoch) Prepare(r *Reconciler) (update *string, replace bool, replaceBaseline string, err error) {
	res := r.Reconcile(e.Snapshot)
	switch res.Action {
	case Unchanged:
		return nil, false, "", nil
	case Updated:
		e.Snapshot = res.Snapshot
		t := res.Text
		return &t, false, "", nil
	case Replace:
		base, snap, err := r.Replace(e.Snapshot)
		if err != nil {
			return nil, true, "", err
		}
		e.Baseline = base
		e.Snapshot = snap
		return nil, true, base, nil
	}
	return nil, false, "", nil
}
