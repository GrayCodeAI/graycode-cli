package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateAndGet(t *testing.T) {
	s, _ := New("")
	e, err := s.Create(KindMemory, "user-prefers-rust", "prefers Rust over Go", "user stated in session", "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if e.Version != 1 {
		t.Fatalf("version = %d", e.Version)
	}
	got, ok := s.Get(KindMemory, "user-prefers-rust")
	if !ok || got.Content != "prefers Rust over Go" {
		t.Fatalf("get = %+v ok=%v", got, ok)
	}
}

func TestRefineBumpsVersionAndRecordsEvidence(t *testing.T) {
	s, _ := New("")
	_, _ = s.Create(KindSkill, "deploy", "step one", "initial", "s")
	refined, err := s.Refine(KindSkill, "deploy", "step one, step two", "observed new step")
	if err != nil {
		t.Fatal(err)
	}
	if refined.Version != 2 {
		t.Fatalf("version = %d, want 2", refined.Version)
	}
	if refined.Evidence != "observed new step" {
		t.Fatalf("evidence = %q", refined.Evidence)
	}
	hist := s.History()
	if len(hist) != 2 || hist[1].Evidence != "observed new step" {
		t.Fatalf("history = %+v", hist)
	}
}

func TestSnapshotRestoreRollback(t *testing.T) {
	s, _ := New("")
	_, _ = s.Create(KindMemory, "m1", "old value", "e1", "s")
	snap := s.Snapshot()
	_, _ = s.Refine(KindMemory, "m1", "new value", "e2")
	if got, _ := s.Get(KindMemory, "m1"); got.Content != "new value" {
		t.Fatalf("after refine = %q", got.Content)
	}
	if err := s.Restore(snap, "revert bad change"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(KindMemory, "m1")
	if got.Content != "old value" {
		t.Fatalf("after rollback = %q", got.Content)
	}
}

func TestDelete(t *testing.T) {
	s, _ := New("")
	_, _ = s.Create(KindSubagent, "reviewer", "prompt", "e", "s")
	if err := s.Delete(KindSubagent, "reviewer", "no longer used"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(KindSubagent, "reviewer"); ok {
		t.Fatal("entry should be deleted")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s1, _ := New(dir)
	_, _ = s1.Create(KindPrompt, "style", "be concise", "evidence", "s")
	s1.Refine(KindPrompt, "style", "be very concise", "more evidence")

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get(KindPrompt, "style")
	if !ok || got.Version != 2 || got.Content != "be very concise" {
		t.Fatalf("reloaded = %+v ok=%v", got, ok)
	}
	if len(s2.History()) != 2 {
		t.Fatalf("history not persisted: %d", len(s2.History()))
	}
}

func TestInvalidKind(t *testing.T) {
	s, _ := New("")
	if _, err := s.Create("bogus", "x", "y", "", ""); err == nil || !strings.Contains(err.Error(), "invalid kind") {
		t.Fatalf("err = %v", err)
	}
}

func TestListSorted(t *testing.T) {
	s, _ := New("")
	_, _ = s.Create(KindMemory, "zeta", "1", "", "")
	_, _ = s.Create(KindMemory, "alpha", "2", "", "")
	items := s.List(KindMemory)
	if len(items) != 2 || items[0].Title != "alpha" || items[1].Title != "zeta" {
		t.Fatalf("items = %+v", items)
	}
}

func TestFilePersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	_, _ = s.Create(KindSkill, "s1", "c", "", "")
	if _, err := os.Stat(filepath.Join(dir, "harness.json")); err != nil {
		t.Fatalf("harness.json not written: %v", err)
	}
}
