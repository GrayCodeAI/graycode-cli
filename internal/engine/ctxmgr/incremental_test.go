package ctxmgr

import (
	"strings"
	"sync"
	"testing"
)

type valSource struct {
	mu    sync.Mutex
	value string
}

func (v *valSource) get() (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.value, nil
}

func (v *valSource) set(s string) {
	v.mu.Lock()
	v.value = s
	v.mu.Unlock()
}

func TestIncrementalInitializeBaseline(t *testing.T) {
	mem := &valSource{value: "remembered X"}
	ic, err := NewIncrementalContext([]Section{
		{Key: "memories", Header: "## Relevant Memories", Load: mem.get},
	})
	if err != nil {
		t.Fatalf("NewIncrementalContext: %v", err)
	}
	base, err := ic.Initialize()
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !strings.Contains(base, "## Relevant Memories") || !strings.Contains(base, "remembered X") {
		t.Fatalf("baseline = %q", base)
	}
}

func TestIncrementalUnchanged(t *testing.T) {
	mem := &valSource{value: "remembered X"}
	ic, _ := NewIncrementalContext([]Section{
		{Key: "memories", Header: "## Relevant Memories", Load: mem.get},
	})
	if _, err := ic.Initialize(); err != nil {
		t.Fatal(err)
	}
	msg, replaced, _, err := ic.Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("unexpected replace")
	}
	if msg != nil {
		t.Fatalf("unexpected update %q", *msg)
	}
}

func TestIncrementalUpdateEmitsOnlyChanged(t *testing.T) {
	mem := &valSource{value: "remembered X"}
	ic, _ := NewIncrementalContext([]Section{
		{Key: "memories", Header: "## Relevant Memories", Load: mem.get},
	})
	if _, err := ic.Initialize(); err != nil {
		t.Fatal(err)
	}
	mem.set("remembered Y")
	msg, replaced, _, err := ic.Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("unexpected replace")
	}
	if msg == nil {
		t.Fatal("expected an update")
	}
	if !strings.Contains(*msg, "remembered Y") {
		t.Fatalf("update = %q", *msg)
	}
	// Baseline stays stable across an update.
	if !strings.Contains(ic.Baseline(), "remembered X") {
		t.Fatalf("baseline should remain immutable, got %q", ic.Baseline())
	}
}

func TestIncrementalSnapshotRoundTrip(t *testing.T) {
	mem := &valSource{value: "remembered X"}
	ic, _ := NewIncrementalContext([]Section{
		{Key: "memories", Header: "## Relevant Memories", Load: mem.get},
	})
	if _, err := ic.Initialize(); err != nil {
		t.Fatal(err)
	}
	b, err := ic.SnapshotBytes()
	if err != nil {
		t.Fatalf("SnapshotBytes: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("expected non-empty snapshot")
	}

	// A fresh host restores the snapshot and sees no change.
	ic2, _ := NewIncrementalContext([]Section{
		{Key: "memories", Header: "## Relevant Memories", Load: mem.get},
	})
	if err := ic2.RestoreSnapshot(b); err != nil {
		t.Fatalf("RestoreSnapshot: %v", err)
	}
	msg, replaced, _, err := ic2.Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	if replaced || msg != nil {
		t.Fatalf("restored host should see no change, got replaced=%v msg=%v", replaced, msg)
	}
}

func TestIncrementalSnapshotReflectsChangeAfterRestore(t *testing.T) {
	mem := &valSource{value: "remembered X"}
	ic, _ := NewIncrementalContext([]Section{
		{Key: "memories", Header: "## Relevant Memories", Load: mem.get},
	})
	if _, err := ic.Initialize(); err != nil {
		t.Fatal(err)
	}
	b, _ := ic.SnapshotBytes()

	// Change the value, then restore a fresh host: it must emit an update.
	mem.set("remembered Z")
	ic2, _ := NewIncrementalContext([]Section{
		{Key: "memories", Header: "## Relevant Memories", Load: mem.get},
	})
	if err := ic2.RestoreSnapshot(b); err != nil {
		t.Fatal(err)
	}
	msg, replaced, _, err := ic2.Reconcile()
	if err != nil {
		t.Fatal(err)
	}
	if replaced {
		t.Fatal("unexpected replace")
	}
	if msg == nil || !strings.Contains(*msg, "remembered Z") {
		t.Fatalf("expected update with new value, got %v", msg)
	}
}

func TestIncrementalRequiresSection(t *testing.T) {
	if _, err := NewIncrementalContext(nil); err == nil {
		t.Fatal("expected error for empty sections")
	}
}

func TestRenderSectionOmitsEmptyHeader(t *testing.T) {
	if got := renderSection("## H", ""); got != "" {
		t.Fatalf("empty value should render empty, got %q", got)
	}
	if got := renderSection("", "value"); got != "value" {
		t.Fatalf("empty header should not emit header, got %q", got)
	}
	if got := renderSection("## H", "v"); !strings.HasPrefix(got, "## H") {
		t.Fatalf("expected header prefix, got %q", got)
	}
}
