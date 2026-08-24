package conversationarc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewArcInit(t *testing.T) {
	a := New()
	if a.CurrentPhase != PhaseInit {
		t.Fatalf("phase = %s", a.CurrentPhase)
	}
	if len(a.Goals) != 0 {
		t.Fatal("expected no goals")
	}
	if !a.IsEmpty() {
		t.Fatal("a fresh arc should be empty")
	}
	a.AddGoal("x")
	if a.IsEmpty() {
		t.Fatal("arc with a goal should not be empty")
	}
}

func TestDetectAndAdvancePhase(t *testing.T) {
	a := New()
	if !a.AdvancePhase(DetectPhase("find the bug and write a fix")) {
		t.Fatal("expected phase advance for exploring")
	}
	if a.CurrentPhase != PhaseImplementing {
		t.Fatalf("phase = %s, want implementing", a.CurrentPhase)
	}
	// Non-monotonic: a completed keyword should advance past implementing.
	if !a.AdvancePhase(DetectPhase("done now")) {
		t.Fatal("expected advance to completed")
	}
	if a.CurrentPhase != PhaseCompleted {
		t.Fatalf("phase = %s, want completed", a.CurrentPhase)
	}
}

func TestPhaseDoesNotRegress(t *testing.T) {
	a := New()
	a.CurrentPhase = PhaseImplementing
	if a.AdvancePhase(PhaseExploring) {
		t.Fatal("phase should not regress")
	}
}

func TestAddGoalAdvancesFromInit(t *testing.T) {
	a := New()
	g := a.AddGoal("implement X")
	if a.CurrentPhase != PhaseExploring {
		t.Fatalf("phase = %s, want exploring", a.CurrentPhase)
	}
	if g.Status != GoalPending {
		t.Fatalf("status = %s", g.Status)
	}
}

func TestUpdateGoalStatusCompletion(t *testing.T) {
	a := New()
	g := a.AddGoal("implement X")
	if !a.UpdateGoalStatus(g.ID, GoalCompleted) {
		t.Fatal("update failed")
	}
	if a.Goals[0].Status != GoalCompleted || a.Goals[0].CompletedAt == nil {
		t.Fatalf("goal not completed: %+v", a.Goals[0])
	}
	if a.UpdateGoalStatus("nope", GoalCompleted) {
		t.Fatal("update of unknown id should fail")
	}
}

func TestSummaryByteStable(t *testing.T) {
	a := New()
	a.AddGoal("implement X")
	a.AddDecision("use go", "stdlib")
	a.AddMilestone("prototype done")
	s1 := a.Summary()
	s2 := a.Summary()
	if s1 != s2 {
		t.Fatalf("summary not byte-stable:\n%q\n%q", s1, s2)
	}
	if !strings.Contains(s1, "implement X") {
		t.Fatalf("summary missing goal:\n%s", s1)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := New()
	a.AddGoal("implement X")
	a.CurrentPhase = PhaseImplementing
	if err := a.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("loaded nil")
	}
	if len(loaded.Goals) != 1 || loaded.Goals[0].Description != "implement X" {
		t.Fatalf("loaded goals wrong: %+v", loaded.Goals)
	}
	if loaded.CurrentPhase != PhaseImplementing {
		t.Fatalf("loaded phase = %s", loaded.CurrentPhase)
	}
}

func TestLoadMissingReturnsNil(t *testing.T) {
	loaded, err := Load(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Fatal("expected nil for missing arc")
	}
}

func TestLoadMalformedReturnsNil(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, arcFileName), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Fatal("expected nil for malformed arc")
	}
}
