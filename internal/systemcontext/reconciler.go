package systemcontext

import (
	"encoding/json"
	"sort"
	"strings"
)

// Action is the outcome of reconciling current context against a snapshot.
type Action int

const (
	// Unchanged means no model-visible context change needs to be admitted.
	Unchanged Action = iota
	// Updated means at least one source changed; Text carries the single
	// combined mid-conversation system message to emit.
	Updated
	// Replace means the baseline can no longer be reused and a fresh baseline
	// must be rendered.
	Replace
)

// Result is the outcome of a reconcile operation.
type Result struct {
	Action Action
	Text   string
	// Snapshot is the advanced snapshot to persist when Action == Updated.
	Snapshot *Snapshot
}

// Reconciler compares loaded source values against a durable snapshot and
// produces incremental updates. It is the core of the incremental-context
// pattern: only changed sources render text, so an immutable baseline is
// preserved across turns.
type Reconciler struct {
	ctx *SystemContext
}

// NewReconciler wraps a composed SystemContext.
func NewReconciler(ctx *SystemContext) *Reconciler {
	return &Reconciler{ctx: ctx}
}

// LoadAll returns the current raw value of every source. It is used to build
// an initial baseline.
func (r *Reconciler) LoadAll() (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	entries := r.ctx.observe()
	for _, e := range entries {
		if !e.avail {
			continue
		}
		out[e.key] = e.raw
	}
	return out, nil
}

// RenderBaseline renders the full ordered baseline text for the given values.
func (r *Reconciler) RenderBaseline(values map[string]json.RawMessage) string {
	var b strings.Builder
	for _, e := range r.ordered() {
		raw, ok := values[e.key.String()]
		if !ok {
			continue
		}
		if t := e.base(raw); t != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(t)
		}
	}
	return b.String()
}

// Initialize produces a fresh baseline and its snapshot. If any source is
// unavailable it returns an error so callers never persist an incomplete
// baseline.
func (r *Reconciler) Initialize() (baseline string, snap *Snapshot, err error) {
	entries := r.ctx.observe()
	snap = NewSnapshot()
	for _, e := range entries {
		if !e.avail {
			return "", nil, &InitializationBlocked{Key: e.key}
		}
		snap.Values[e.key] = e.raw
	}
	return r.RenderBaseline(snap.Values), snap, nil
}

// InitializationBlocked reports that an unavailable source blocked baseline
// initialization.
type InitializationBlocked struct{ Key string }

func (e *InitializationBlocked) Error() string {
	return "systemcontext: source " + e.Key + " unavailable during initialization"
}

// Reconcile compares current context against the provided snapshot and returns
// exactly one action.
//
//   - Unchanged: no changes.
//   - Updated: Text is the single combined system message; Snapshot is the
//     advanced snapshot to persist.
//   - Replace: baseline can no longer be reused (a removable source vanished
//     without a removal renderer, or an incompatible transition).
func (r *Reconciler) Reconcile(prev *Snapshot) *Result {
	if prev == nil {
		prev = NewSnapshot()
	}
	entries := r.ctx.observe()

	var parts []string
	next := NewSnapshot()
	changed := false

	for _, e := range entries {
		src := r.source(e.key)
		if src == nil {
			// Source not registered in this context; treat as replacement to be
			// safe rather than silently dropping.
			return &Result{Action: Replace}
		}
		if !e.avail {
			// Unavailable: retain prior effective value (stale-while-revalidate).
			if prevRaw, ok := prev.Values[e.key]; ok {
				next.Values[e.key] = prevRaw
			}
			continue
		}
		prevRaw, had := prev.Values[e.key]
		if !had {
			// New source: emit baseline once.
			if t := src.base(e.raw); t != "" {
				parts = append(parts, t)
			}
			changed = true
		} else if !src.equal(prevRaw, e.raw) {
			if t := src.upd(prevRaw, e.raw); t != "" {
				parts = append(parts, t)
			}
			changed = true
		}
		next.Values[e.key] = e.raw
	}

	// Detect removed sources: a previously-admitted key with no current value.
	// Because sources form a fixed set, a vanished source means the composition
	// changed and the baseline must be rebuilt.
	for key := range prev.Values {
		if _, present := next.Values[key]; present {
			continue
		}
		return &Result{Action: Replace}
	}

	if !changed {
		return &Result{Action: Unchanged}
	}
	return &Result{Action: Updated, Text: strings.Join(parts, "\n\n"), Snapshot: next}
}

// Replace renders a fresh baseline after a baseline-replacing transition (e.g.
// compaction). It reports replacement blocked while previously-admitted context
// is unavailable.
func (r *Reconciler) Replace(prev *Snapshot) (baseline string, snap *Snapshot, err error) {
	if prev != nil {
		for key := range prev.Values {
			if r.source(key) == nil {
				return "", nil, &InitializationBlocked{Key: key}
			}
		}
	}
	return r.Initialize()
}

// source returns the typedSource for a stable key string.
func (r *Reconciler) source(key string) *typedSource {
	for i := range r.ctx.sources {
		if r.ctx.sources[i].key.String() == key {
			return &r.ctx.sources[i]
		}
	}
	return nil
}

// ordered returns the sources sorted by stable key for deterministic rendering.
func (r *Reconciler) ordered() []typedSource {
	out := make([]typedSource, len(r.ctx.sources))
	copy(out, r.ctx.sources)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].key.String() < out[j].key.String()
	})
	return out
}
