package engine

import (
	"path/filepath"
	"testing"
)

func TestSelfImproverLearnAndForPrompt(t *testing.T) {
	si := &SelfImprover{Path: filepath.Join(t.TempDir(), "self-improve.json")}
	si.Learn("write failed", "wrong encoding", "always verify encoding", "code")
	si.Learn("test flaked", "race", "use -race", "test")

	out := si.ForPrompt(5)
	if out == "" {
		t.Fatal("expected lessons in prompt")
	}
	if len(si.Lessons("code")) != 1 {
		t.Fatalf("expected 1 code lesson, got %d", len(si.Lessons("code")))
	}
}

func TestSelfImproverBounded(t *testing.T) {
	si := &SelfImprover{Path: filepath.Join(t.TempDir(), "self-improve.json")}
	for i := 0; i < maxSelfImproveEntries+50; i++ {
		si.Learn("x", "y", "z", "code")
	}
	if len(si.Entries) != maxSelfImproveEntries {
		t.Fatalf("expected %d entries, got %d", maxSelfImproveEntries, len(si.Entries))
	}
}

func TestSelfImproverNilSafe(t *testing.T) {
	var si *SelfImprover
	si.Learn("x", "y", "z", "code") // must not panic
	if out := si.ForPrompt(5); out != "" {
		t.Fatalf("expected empty prompt for nil improver, got %q", out)
	}
	if lessons := si.Lessons("code"); lessons != nil {
		t.Fatalf("expected nil lessons for nil improver")
	}
}

func TestSessionLearnCallback(t *testing.T) {
	si := &SelfImprover{Path: filepath.Join(t.TempDir(), "self-improve.json")}
	s := &Session{}
	s.SetLearnFn(si.Learn)
	s.Learn("bash failed", "bad path", "quote paths", "tool_failure")

	if len(si.Entries) != 1 || si.Entries[0].Category != "tool_failure" {
		t.Fatalf("expected 1 persisted lesson, got %+v", si.Entries)
	}
}

func TestSessionLearnNilSafe(t *testing.T) {
	s := &Session{}
	s.Learn("x", "y", "z", "code") // no callback, must not panic
}
