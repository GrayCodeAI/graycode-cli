package engine

import (
	"testing"
	"time"
)

func TestBudgetTurnLimit(t *testing.T) {
	b := NewAutonomousBudget(AutonomousLimits{MaxTurns: 3})
	if b.Running() != true {
		t.Fatal("should start running")
	}
	b.RecordTurn()
	b.RecordTurn()
	if r := b.RecordTurn(); r != ReasonBudgetTurns {
		t.Fatalf("reason = %q, want budget_limited:turns", r)
	}
	if b.Running() {
		t.Fatal("should be stopped")
	}
	if s := b.Status(); s.TurnsUsed != 3 || s.Reason != ReasonBudgetTurns {
		t.Fatalf("status = %+v", s)
	}
}

func TestBudgetTokenLimit(t *testing.T) {
	b := NewAutonomousBudget(AutonomousLimits{MaxTokens: 1000})
	b.RecordToken(600)
	if r := b.RecordToken(500); r != ReasonBudgetTokens {
		t.Fatalf("reason = %q, want budget_limited:tokens", r)
	}
}

func TestBudgetTimeLimit(t *testing.T) {
	b := NewAutonomousBudget(AutonomousLimits{MaxTime: 20 * time.Millisecond})
	time.Sleep(30 * time.Millisecond)
	if r := b.Check(); r != ReasonBudgetTime {
		t.Fatalf("reason = %q, want budget_limited:time", r)
	}
}

func TestGatePassedBeatsBudgetsOnlyIfRunning(t *testing.T) {
	b := NewAutonomousBudget(AutonomousLimits{MaxTurns: 2})
	b.RecordTurn()
	b.MarkGatePassed()
	if b.StopReason() != ReasonGatePassed {
		t.Fatalf("reason = %q, want gate_passed", b.StopReason())
	}
	// After a terminal reason, budgets do not override it.
	b.RecordTurn()
	if b.StopReason() != ReasonGatePassed {
		t.Fatalf("terminal should stick, got %q", b.StopReason())
	}
}

func TestErrorSticks(t *testing.T) {
	b := NewAutonomousBudget(AutonomousLimits{MaxTurns: 10})
	b.MarkError("boom")
	if b.StopReason() != ReasonError || b.Status().LastError != "boom" {
		t.Fatalf("status = %+v", b.Status())
	}
	if b.Running() {
		t.Fatal("should stop on error")
	}
}

func TestFirstBudgetHitWins(t *testing.T) {
	b := NewAutonomousBudget(AutonomousLimits{MaxTurns: 2, MaxTokens: 100})
	b.RecordTurn()
	b.RecordToken(120) // tokens hit first -> terminal reason sticks
	b.RecordTurn()     // turns now also exceeded, but reason is already terminal
	if b.StopReason() != ReasonBudgetTokens {
		t.Fatalf("reason = %q, want budget_limited:tokens (first hit wins)", b.StopReason())
	}
}

func TestUnlimitedRun(t *testing.T) {
	b := NewAutonomousBudget(AutonomousLimits{})
	for i := 0; i < 100; i++ {
		b.RecordTurn()
		b.RecordToken(1)
		b.RecordContinuation()
	}
	if !b.Running() || b.StopReason() != ReasonRunning {
		t.Fatalf("unlimited run should keep running, got %q", b.StopReason())
	}
}
