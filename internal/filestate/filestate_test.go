package filestate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewindBasicFlow(t *testing.T) {
	state := t.TempDir()
	t.Setenv("HAWK_STATE_DIR", state)
	work := t.TempDir()
	f := filepath.Join(work, "code.go")
	if err := os.WriteFile(f, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tr, err := NewTracker("sess-1", false)
	if err != nil {
		t.Fatal(err)
	}
	tr.BeginPrompt(0)
	tr.SetTouchResult(f, "v1\n", "v2\n")
	if err := os.WriteFile(f, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tr.EndPrompt(); err != nil {
		t.Fatal(err)
	}
	if tr.Len() != 1 {
		t.Fatalf("points = %d", tr.Len())
	}

	// Prompt 1 edits again.
	tr.BeginPrompt(1)
	tr.SetTouchResult(f, "v2\n", "v3\n")
	_ = os.WriteFile(f, []byte("v3\n"), 0o644)
	_ = tr.EndPrompt()

	// Rewind to prompt 0: plan restores v1.
	plan, err := tr.RewindTo(0)
	if err != nil {
		t.Fatal(err)
	}
	if plan[f] != "v1\n" {
		t.Fatalf("plan = %q", plan[f])
	}
	// Later history truncated.
	if tr.Len() != 0 {
		t.Fatalf("len after rewind = %d", tr.Len())
	}
}

func TestRewindSkipsUnchangedFiles(t *testing.T) {
	state := t.TempDir()
	t.Setenv("HAWK_STATE_DIR", state)
	work := t.TempDir()
	f := filepath.Join(work, "a.txt")
	_ = os.WriteFile(f, []byte("same"), 0o644)

	tr, _ := NewTracker("s", false)
	tr.BeginPrompt(0)
	tr.SetTouchResult(f, "same", "same") // touched but unchanged
	_ = tr.EndPrompt()

	plan, err := tr.RewindTo(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 0 {
		t.Fatalf("plan should skip unchanged files: %v", plan)
	}
}

func TestDurableRehydration(t *testing.T) {
	state := t.TempDir()
	t.Setenv("HAWK_STATE_DIR", state)
	work := t.TempDir()
	f := filepath.Join(work, "x.txt")
	_ = os.WriteFile(f, []byte("one"), 0o644)

	tr, err := NewTracker("durable-session", true)
	if err != nil {
		t.Fatal(err)
	}
	tr.BeginPrompt(7)
	tr.SetTouchResult(f, "one", "two")
	_ = os.WriteFile(f, []byte("two"), 0o644)
	if err := tr.EndPrompt(); err != nil {
		t.Fatal(err)
	}

	// A brand-new tracker (process restart) rehydrates from disk.
	tr2, err := NewTracker("durable-session", true)
	if err != nil {
		t.Fatal(err)
	}
	if tr2.Len() != 1 {
		t.Fatalf("rehydrated points = %d", tr2.Len())
	}
	plan, err := tr2.RewindTo(7)
	if err != nil {
		t.Fatal(err)
	}
	if plan[f] != "one" {
		t.Fatalf("plan = %q", plan[f])
	}
}

func TestSanitizeSessionID(t *testing.T) {
	hostile := "../../evil/../../id with spaces!"
	sanitized := sanitizeSessionID(hostile)
	if strings.Contains(sanitized, "..") || strings.Contains(sanitized, "/") ||
		strings.Contains(sanitized, " ") {
		t.Fatalf("sanitized leaks traversal: %q", sanitized)
	}
	if sanitizeSessionID("") != "" {
		t.Fatal("empty id must stay empty")
	}
	// Same input -> same output (stable store dirs).
	if sanitizeSessionID(hostile) != sanitized {
		t.Fatal("sanitize not deterministic")
	}
}

func TestEvictionCap(t *testing.T) {
	state := t.TempDir()
	t.Setenv("HAWK_STATE_DIR", state)
	tr, _ := NewTracker("cap-sess", false)
	for i := 0; i < defaultCap+10; i++ {
		tr.BeginPrompt(i)
		tr.SetTouchResult("/unused", "a", "b")
		if err := tr.EndPrompt(); err != nil {
			t.Fatal(err)
		}
	}
	if tr.Len() != defaultCap {
		t.Fatalf("len = %d, want cap %d", tr.Len(), defaultCap)
	}
	// Oldest points evicted: index 0 no longer available.
	if _, err := tr.RewindTo(0); err == nil {
		t.Fatal("expected ErrNoRewindPoint for evicted index")
	} else if err != ErrNoRewindPoint {
		t.Fatalf("err = %v", err)
	}
	// Newest point still available.
	if _, err := tr.RewindTo(defaultCap + 9); err != nil {
		t.Fatalf("newest point missing: %v", err)
	}
}

func TestNoTouchNoPoint(t *testing.T) {
	tr, _ := NewTracker("empty", false)
	tr.BeginPrompt(0)
	if err := tr.EndPrompt(); err != nil {
		t.Fatal(err)
	}
	if tr.Len() != 0 {
		t.Fatalf("empty window stored a point: %d", tr.Len())
	}
}

func TestRewindUnknownIndex(t *testing.T) {
	tr, _ := NewTracker("none", false)
	if _, err := tr.RewindTo(42); err != ErrNoRewindPoint {
		t.Fatalf("err = %v", err)
	}
}
