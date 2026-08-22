package systemcontext

import (
	"encoding/json"
	"sync"
	"testing"
)

// statefulSource is a source whose value can be changed by the test.
type statefulSource struct {
	mu        sync.Mutex
	value     string
	loadErr   bool
	removable bool
}

func newStateful(value string) *statefulSource {
	return &statefulSource{value: value}
}

func (s *statefulSource) load() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr {
		return "", errLoad
	}
	return s.value, nil
}

func (s *statefulSource) set(v string) {
	s.mu.Lock()
	s.value = v
	s.mu.Unlock()
}

var errLoad = &jsonError{"load failed"}

type jsonError struct{ s string }

func (e *jsonError) Error() string { return e.s }

func strCodec() Codec[string] {
	return JSONCodec(func(a, b string) bool { return a == b })
}

func makeSrc(s *statefulSource) Source[string] {
	return Source[string]{
		Key:      NewKey("test", "value"),
		Codec:    strCodec(),
		Load:     s.load,
		Baseline: func(v string) string { return "BASE[" + v + "]" },
		Update:   func(_, cur string) string { return "UPDATE[" + cur + "]" },
	}
}

func TestInitializeRendersDeterministicBaseline(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)

	base, snap, err := r.Initialize()
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if base != "BASE[v1]" {
		t.Fatalf("baseline = %q, want %q", base, "BASE[v1]")
	}
	if snap == nil || snap.Values["test/value"] == nil {
		t.Fatalf("snapshot missing value")
	}
	if _, err := snap.Marshal(); err != nil {
		t.Fatalf("snapshot marshal: %v", err)
	}
}

func TestReconcileUnchanged(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	_, snap, _ := r.Initialize()

	res := r.Reconcile(snap)
	if res.Action != Unchanged {
		t.Fatalf("action = %v, want Unchanged", res.Action)
	}
}

func TestReconcileUpdate(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	_, snap, _ := r.Initialize()

	s.set("v2")
	res := r.Reconcile(snap)
	if res.Action != Updated {
		t.Fatalf("action = %v, want Updated", res.Action)
	}
	if res.Text != "UPDATE[v2]" {
		t.Fatalf("text = %q, want %q", res.Text, "UPDATE[v2]")
	}
	if res.Snapshot == nil {
		t.Fatal("advanced snapshot missing")
	}

	// A subsequent reconcile with the advanced snapshot is unchanged.
	again := r.Reconcile(res.Snapshot)
	if again.Action != Unchanged {
		t.Fatalf("second action = %v, want Unchanged", again.Action)
	}
}

func TestReconcileNewSourceEmitsBaselineOnce(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	// Start with an empty snapshot: the source is new.
	res := r.Reconcile(NewSnapshot())
	if res.Action != Updated {
		t.Fatalf("action = %v, want Updated", res.Action)
	}
	if res.Text != "BASE[v1]" {
		t.Fatalf("text = %q, want %q", res.Text, "BASE[v1]")
	}
}

func TestUnavailableRetainsPriorValue(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	_, snap, _ := r.Initialize()

	// Mark the source unavailable; reconcile should retain the prior value.
	s.loadErr = true
	res := r.Reconcile(snap)
	if res.Action != Unchanged {
		t.Fatalf("action = %v, want Unchanged (stale-while-revalidate)", res.Action)
	}
}

func TestInitializeBlocksOnUnavailable(t *testing.T) {
	s := newStateful("v1")
	s.loadErr = true
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	_, _, err := r.Initialize()
	if err == nil {
		t.Fatal("expected InitializationBlocked")
	}
	if _, ok := err.(*InitializationBlocked); !ok {
		t.Fatalf("err = %T, want *InitializationBlocked", err)
	}
}

func TestRemovableSourceRemovalRendersRemoval(t *testing.T) {
	// A dynamic context where sources can appear/disappear requires a host that
	// rebuilds the SystemContext. Here we simulate by using two sources and a
	// snapshot that records both, then reconcile with a context missing one.
	a := newStateful("a1")
	b := newStateful("b1")
	ctxAB := NewAll(sourceA(a), sourceB(b))
	rAB := NewReconciler(ctxAB)
	_, snap, _ := rAB.Initialize()

	// Rebuild with only source A. Source B's key is still in the snapshot but
	// absent from the new context.
	ctxA := New(sourceA(a))
	rA := NewReconciler(ctxA)
	res := rA.Reconcile(snap)
	if res.Action != Replace {
		// Composition change => replacement required.
		t.Fatalf("action = %v, want Replace", res.Action)
	}
}

func sourceA(s *statefulSource) Source[string] {
	return Source[string]{
		Key:      NewKey("test", "a"),
		Codec:    strCodec(),
		Load:     s.load,
		Baseline: func(v string) string { return "A[" + v + "]" },
		Update:   func(_, cur string) string { return "A-UPD[" + cur + "]" },
	}
}

func sourceB(s *statefulSource) Source[string] {
	return Source[string]{
		Key:      NewKey("test", "b"),
		Codec:    strCodec(),
		Load:     s.load,
		Baseline: func(v string) string { return "B[" + v + "]" },
		Update:   func(_, cur string) string { return "B-UPD[" + cur + "]" },
	}
}

func TestEpochPrepareUpdateAndBaselineStable(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	base, snap, _ := r.Initialize()
	ep := NewEpoch(base, snap)
	origBase := ep.Baseline

	s.set("v2")
	upd, replace, _, err := ep.Prepare(r)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if replace {
		t.Fatal("unexpected replace")
	}
	if upd == nil || *upd != "UPDATE[v2]" {
		t.Fatalf("update = %v, want UPDATE[v2]", upd)
	}
	// Baseline must remain unchanged across an update.
	if ep.Baseline != origBase {
		t.Fatalf("baseline changed on update")
	}

	// Next prepare is unchanged.
	upd2, replace2, _, err := ep.Prepare(r)
	if err != nil || upd2 != nil || replace2 {
		t.Fatalf("second prepare = (%v,%v,%v), want unchanged", upd2, replace2, err)
	}
}

func TestEpochReplaceFoldsFreshBaseline(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	base, snap, _ := r.Initialize()
	ep := NewEpoch(base, snap)

	// A compaction-like transition replaces the baseline.
	newBase, newSnap, err := r.Replace(ep.Snapshot)
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if newBase != "BASE[v1]" {
		t.Fatalf("new baseline = %q", newBase)
	}
	ep.Baseline = newBase
	ep.Snapshot = newSnap
	if ep.Baseline != "BASE[v1]" {
		t.Fatalf("epoch baseline not replaced")
	}
}

func TestDuplicateKeyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate key")
		}
	}()
	s := newStateful("v1")
	NewAll(makeSrc(s), makeSrc(s))
}

func TestSnapshotMarshalRoundTrip(t *testing.T) {
	s := newStateful("v1")
	ctx := New(makeSrc(s))
	r := NewReconciler(ctx)
	_, snap, _ := r.Initialize()

	b, err := snap.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var restored Snapshot
	if err := restored.Unmarshal(b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(restored.Values["test/value"]) == "" {
		t.Fatal("round-trip lost value")
	}

	// Reconciling against the restored snapshot should be unchanged.
	res := r.Reconcile(&restored)
	if res.Action != Unchanged {
		t.Fatalf("action against restored snapshot = %v, want Unchanged", res.Action)
	}
}

func TestJSONEqualityIgnoresWhitespace(t *testing.T) {
	// A codec with no Equal falls back to normalized JSON comparison.
	c := Codec[json.RawMessage]{
		Encode: func(v json.RawMessage) (json.RawMessage, error) { return v, nil },
		Decode: func(b json.RawMessage) (json.RawMessage, error) { return b, nil },
	}
	a := json.RawMessage(`{"x":1}`)
	b := json.RawMessage(`{"x": 1}`)
	if !equalJSON(c, a, b) {
		t.Fatal("expected normalized JSON equality")
	}
	// Different values must compare unequal.
	c2 := json.RawMessage(`{"x":2}`)
	if equalJSON(c, a, c2) {
		t.Fatal("expected inequality for different values")
	}
}
