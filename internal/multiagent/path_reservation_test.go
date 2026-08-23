package mission

import (
	"strings"
	"testing"
)

func TestReserveAndHolder(t *testing.T) {
	l := NewPathReservationLedger()
	if err := l.Reserve("agent-1", "feat-1", []string{"internal/a.go", "internal/b.go"}); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if h := l.Holder("internal/a.go"); h != "agent-1" {
		t.Fatalf("holder = %q", h)
	}
	// Idempotent re-reserve by same agent.
	if err := l.Reserve("agent-1", "feat-1", []string{"internal/a.go"}); err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	held := l.HeldBy("agent-1")
	if len(held) != 2 {
		t.Fatalf("held = %v", held)
	}
}

func TestReserveConflictAllOrNothing(t *testing.T) {
	l := NewPathReservationLedger()
	if err := l.Reserve("a", "f1", []string{"x.go"}); err != nil {
		t.Fatal(err)
	}
	err := l.Reserve("b", "f2", []string{"y.go", "x.go"})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "x.go (held by a)") {
		t.Fatalf("error = %v", err)
	}
	// All-or-nothing: y.go must NOT have been claimed by b.
	if h := l.Holder("y.go"); h != "" {
		t.Fatalf("partial claim leaked: y.go held by %q", h)
	}
	if h := l.Holder("x.go"); h != "a" {
		t.Fatalf("x.go holder changed: %q", h)
	}
}

func TestReleaseFreesPaths(t *testing.T) {
	l := NewPathReservationLedger()
	_ = l.Reserve("a", "f1", []string{"p/q.go"})
	if n := l.Release("a"); n != 1 {
		t.Fatalf("released = %d", n)
	}
	if h := l.Holder("p/q.go"); h != "" {
		t.Fatalf("still held by %q", h)
	}
	// Another agent can now claim it.
	if err := l.Reserve("b", "f2", []string{"p/q.go"}); err != nil {
		t.Fatalf("claim after release: %v", err)
	}
}

func TestDetectFileOverlaps(t *testing.T) {
	handoffs := map[string][]string{
		"feat-b": {"cmd/main.go", "internal/shared.go"},
		"feat-a": {"internal/a.go", "internal/shared.go"},
		"feat-c": {"internal/c.go"},
	}
	overlaps := DetectFileOverlaps(handoffs)
	if len(overlaps) != 1 {
		t.Fatalf("overlaps = %+v", overlaps)
	}
	o := overlaps[0]
	if o.FeatureA != "feat-a" || o.FeatureB != "feat-b" {
		t.Fatalf("pair = %s/%s", o.FeatureA, o.FeatureB)
	}
	if len(o.Paths) != 1 || o.Paths[0] != "internal/shared.go" {
		t.Fatalf("paths = %v", o.Paths)
	}
}

func TestDetectFileOverlapsNoneAndMultiPair(t *testing.T) {
	if got := DetectFileOverlaps(map[string][]string{"a": {"x"}, "b": {"y"}}); len(got) != 0 {
		t.Fatalf("expected no overlaps, got %+v", got)
	}
	got := DetectFileOverlaps(map[string][]string{
		"a": {"shared", "only-a"},
		"b": {"shared"},
		"c": {"shared"},
	})
	if len(got) != 3 { // a-b, a-c, b-c all share "shared"
		t.Fatalf("overlaps = %+v", got)
	}
	for _, o := range got {
		if len(o.Paths) != 1 || o.Paths[0] != "shared" {
			t.Fatalf("pair paths wrong: %+v", o)
		}
	}
}

func TestCanonicalPathNormalization(t *testing.T) {
	l := NewPathReservationLedger()
	_ = l.Reserve("a", "f", []string{"./internal/x.go"})
	if h := l.Holder("internal/x.go"); h != "a" {
		t.Fatalf("./ prefix not normalized: holder=%q", h)
	}
}
