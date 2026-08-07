package startup

import (
	"testing"
)

func TestMarkAndEndPhase(t *testing.T) {
	Reset()

	MarkPhase("init")
	EndPhase("init")

	phases := GetPhases()
	if len(phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(phases))
	}
	p := phases[0]
	if p.Name != "init" {
		t.Errorf("phase name = %q, want init", p.Name)
	}
	if p.EndTime.IsZero() {
		t.Error("phase EndTime should be set after EndPhase")
	}
	if p.Duration <= 0 {
		t.Errorf("phase Duration = %v, want > 0", p.Duration)
	}
}

func TestEndPhase_UnmatchedIsNoop(t *testing.T) {
	Reset()
	MarkPhase("a")
	// Ending a phase that was never marked must not panic or corrupt state.
	EndPhase("nonexistent")
	EndPhase("a")

	if got := len(GetPhases()); got != 1 {
		t.Errorf("expected 1 phase, got %d", got)
	}
}

func TestEndPhase_OnlyClosesLatestOpenMatch(t *testing.T) {
	Reset()
	MarkPhase("a")
	MarkPhase("b")
	MarkPhase("a") // reopen

	EndPhase("a") // closes the latest open "a"

	phases := GetPhases()
	if len(phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(phases))
	}
	if phases[2].EndTime.IsZero() {
		t.Error("latest 'a' phase should be closed")
	}
	// The first "a" (index 0) must remain open.
	if !phases[0].EndTime.IsZero() {
		t.Error("first 'a' phase should remain open")
	}
}

func TestReset(t *testing.T) {
	Reset()
	MarkPhase("x")
	EndPhase("x")
	if len(GetPhases()) != 1 {
		t.Fatal("setup failed")
	}

	Reset()
	if got := GetPhases(); len(got) != 0 {
		t.Errorf("after Reset, expected 0 phases, got %d", len(got))
	}
}

func TestGetPhases_ReturnsCopy(t *testing.T) {
	Reset()
	MarkPhase("a")
	phases := GetPhases()
	// Mutating the returned slice must not affect internal state.
	phases[0].Name = "mutated"
	if got := GetPhases()[0].Name; got != "a" {
		t.Errorf("GetPhases should return a copy, got %q", got)
	}
}

func TestTotalTime(t *testing.T) {
	Reset()
	total := TotalTime()
	if total < 0 {
		t.Errorf("TotalTime() = %v, want >= 0", total)
	}
}
