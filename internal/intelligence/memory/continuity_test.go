package memory

import (
	"testing"
)

func TestContinuityTrackerBasic(t *testing.T) {
	ct := NewContinuityTracker("")

	ct.StartSession("session1", true)
	ct.RecordMemoryUse(5, 200)
	ct.EndSession(true)

	r := ct.Report()
	if r.TotalSessions != 1 {
		t.Errorf("TotalSessions = %d, want 1", r.TotalSessions)
	}
	if r.SessionsWithMemory != 1 {
		t.Errorf("SessionsWithMemory = %d, want 1", r.SessionsWithMemory)
	}
	if r.SuccessRate != 1.0 {
		t.Errorf("SuccessRate = %f, want 1.0", r.SuccessRate)
	}
	if r.AvgScore <= 0 {
		t.Errorf("AvgScore = %f, want > 0", r.AvgScore)
	}
}

func TestContinuityTrackerMultipleSessions(t *testing.T) {
	ct := NewContinuityTracker("")

	// Session with memory, success
	ct.StartSession("s1", true)
	ct.RecordMemoryUse(3, 100)
	ct.EndSession(true)

	// Session without memory, failure
	ct.StartSession("s2", false)
	ct.RecordReExplanation()
	ct.RecordReExplanation()
	ct.EndSession(false)

	r := ct.Report()
	if r.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", r.TotalSessions)
	}
	if r.SessionsWithMemory != 1 {
		t.Errorf("SessionsWithMemory = %d, want 1", r.SessionsWithMemory)
	}
	if r.SuccessRate != 0.5 {
		t.Errorf("SuccessRate = %f, want 0.5", r.SuccessRate)
	}
	if r.AvgReExplanations != 1.0 {
		t.Errorf("AvgReExplanations = %f, want 1.0", r.AvgReExplanations)
	}
}

func TestContinuityTrackerFormatSummary(t *testing.T) {
	ct := NewContinuityTracker("")
	if s := ct.FormatSummary(); s != "" {
		t.Errorf("expected empty summary with no sessions, got %q", s)
	}

	ct.StartSession("s1", true)
	ct.RecordMemoryUse(2, 50)
	ct.EndSession(true)

	s := ct.FormatSummary()
	if s == "" {
		t.Error("expected non-empty summary after session")
	}
}

func TestContinuityTrackerEmpty(t *testing.T) {
	ct := NewContinuityTracker("")
	r := ct.Report()
	if r.TotalSessions != 0 {
		t.Errorf("expected 0 sessions")
	}
	if r.AvgScore != 0 {
		t.Errorf("expected 0 avg score")
	}
}
