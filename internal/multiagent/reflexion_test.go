package mission

import (
	"strings"
	"testing"
)

func TestReflect_TestFailure(t *testing.T) {
	feat := &Feature{ID: "f1"}
	h := &Handoff{Summary: "impl added but tests fail", TestsPassed: false}
	r := Reflect(feat, h, 1)
	if r.FeatureID != "f1" || r.Attempt != 1 {
		t.Fatalf("identity fields wrong: %+v", r)
	}
	if r.TestsPassed {
		t.Error("TestsPassed must be false")
	}
	if r.WhatWentWrong != "impl added but tests fail" {
		t.Errorf("WhatWentWrong = %q", r.WhatWentWrong)
	}
	if !strings.Contains(r.WhatToTryNext, "fix the root cause") {
		t.Errorf("WhatToTryNext = %q", r.WhatToTryNext)
	}
}

func TestReflect_Uncommitted(t *testing.T) {
	feat := &Feature{ID: "f1"}
	// Tests passed but nothing committed: guidance should say commit.
	r := Reflect(feat, &Handoff{Summary: "works", TestsPassed: true}, 0)
	if r.TestsPassed != true {
		t.Error("TestsPassed must be true")
	}
	if !strings.Contains(r.WhatToTryNext, "commit") {
		t.Errorf("WhatToTryNext = %q, want commit guidance", r.WhatToTryNext)
	}
}

func TestReflect_Deterministic(t *testing.T) {
	feat := &Feature{ID: "f1"}
	h := &Handoff{Summary: "same failure", TestsPassed: false}
	a := Reflect(feat, h, 2)
	b := Reflect(feat, h, 2)
	if a.WhatWentWrong != b.WhatWentWrong || a.WhatToTryNext != b.WhatToTryNext {
		t.Error("Reflect must be deterministic for the same input")
	}
}

func TestReflect_TruncatesLongSummary(t *testing.T) {
	feat := &Feature{ID: "f1"}
	long := strings.Repeat("x", 2000)
	r := Reflect(feat, &Handoff{Summary: long, TestsPassed: false}, 0)
	if len(r.WhatWentWrong) > 503 { // 500 + "..."
		t.Errorf("WhatWentWrong not truncated: %d", len(r.WhatWentWrong))
	}
}

func TestReflexionStore_RecordLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewReflexionStore(dir)
	feat := &Feature{ID: "f1"}

	if err := store.Record(Reflect(feat, &Handoff{Summary: "fail 1", TestsPassed: false}, 1)); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Reflect(feat, &Handoff{Summary: "fail 2", TestsPassed: false}, 2)); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := store.Load("f1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Load returned %d reflexions, want 2", len(got))
	}
	if got[0].Attempt != 1 || got[1].Attempt != 2 {
		t.Errorf("order wrong: attempts %d, %d", got[0].Attempt, got[1].Attempt)
	}
	if got[0].FeatureID != "f1" || got[1].WhatWentWrong != "fail 2" {
		t.Errorf("content wrong: %+v", got)
	}
}

func TestReflexionStore_LoadMissing(t *testing.T) {
	store := NewReflexionStore(t.TempDir())
	got, err := store.Load("nonexistent")
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty for missing feature, got %d", len(got))
	}
}

func TestAttemptFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   int
	}{
		{"graycode-mission/m1/f1/attempt-1", 1},
		{"graycode-mission/m1/f1/attempt-3", 3},
		{"graycode-mission/m1/f1/attempt-12", 12},
		{"graycode-mission/m1/f1", 0},
		{"", 0},
		{"graycode-mission/m1/f1/attempt-", 0},
	}
	for _, tc := range tests {
		if got := attemptFromBranch(tc.branch); got != tc.want {
			t.Errorf("attemptFromBranch(%q) = %d, want %d", tc.branch, got, tc.want)
		}
	}
}
