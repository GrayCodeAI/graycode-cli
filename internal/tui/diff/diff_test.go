package diff

import (
	"strings"
	"testing"
)

func TestChangedNoChange(t *testing.T) {
	prev := []string{"a", "b", "c"}
	next := []string{"a", "b", "c"}
	r := Changed(prev, next)
	if r.Changed {
		t.Fatal("identical frames must not be reported as changed")
	}
}

func TestChangedSingleLine(t *testing.T) {
	prev := []string{"a", "b", "c"}
	next := []string{"a", "B", "c"}
	r := Changed(prev, next)
	if !r.Changed || r.First != 1 || r.Last != 1 {
		t.Fatalf("single-line change range = %+v, want {First:1 Last:1}", r)
	}
}

func TestChangedAppend(t *testing.T) {
	prev := []string{"a", "b"}
	next := []string{"a", "b", "c", "d"}
	r := Changed(prev, next)
	if !r.Changed || !r.Appended || r.First != 2 || r.Last != 3 {
		t.Fatalf("append range = %+v, want {First:2 Last:3 Appended:true}", r)
	}
}

func TestChangedShrink(t *testing.T) {
	prev := []string{"a", "b", "c"}
	next := []string{"a", "X"}
	r := Changed(prev, next)
	if !r.Changed || r.First != 1 || r.Last != 1 {
		t.Fatalf("shrink range = %+v, want {First:1 Last:1}", r)
	}
}

func TestSynchronizedRenderEmitsOnlyChanged(t *testing.T) {
	prev := []string{"line0", "line1", "line2"}
	next := []string{"line0", "CHANGED", "line2"}
	var out strings.Builder
	n, err := SynchronizedRender(&out, prev, next)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 line emitted, got %d", n)
	}
	s := out.String()
	if !strings.Contains(s, "\x1b[?2026h") || !strings.Contains(s, "\x1b[?2026l") {
		t.Fatal("render must wrap output in synchronized sequences")
	}
	if !strings.Contains(s, "CHANGED") {
		t.Fatal("render must contain the changed line")
	}
	if strings.Contains(s, "line0") {
		t.Fatal("render must not re-emit unchanged lines")
	}
}

func TestSynchronizedRenderNoChangeWritesNothing(t *testing.T) {
	prev := []string{"a", "b"}
	next := []string{"a", "b"}
	var out strings.Builder
	n, err := SynchronizedRender(&out, prev, next)
	if err != nil || n != 0 {
		t.Fatalf("no-change render: n=%d err=%v, want 0,nil", n, err)
	}
	if out.Len() != 0 {
		t.Fatalf("no-change render must write nothing, got %q", out.String())
	}
}
