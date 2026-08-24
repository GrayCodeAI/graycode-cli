package engine

import (
	"time"
)

// Bounded autonomous budget tracking, adopted from Prime Agent's autonomous
// mode: the agent runs within configured turn/token/time/continuation budgets
// and a status machine reports WHY a run stopped — budget_limited (with the
// specific budget hit) vs gate_passed vs error — so completion is honest
// ("a passed gate checks only what that gate verifies; reaching a limit does
// not imply task success").

// AutonomousReason classifies why a bounded run stopped.
type AutonomousReason string

const (
	ReasonRunning       AutonomousReason = "running"
	ReasonBudgetTokens  AutonomousReason = "budget_limited:tokens"
	ReasonBudgetTurns   AutonomousReason = "budget_limited:turns"
	ReasonBudgetTime    AutonomousReason = "budget_limited:time"
	ReasonBudgetConcurr AutonomousReason = "budget_limited:continuations"
	ReasonGatePassed    AutonomousReason = "gate_passed"
	ReasonError         AutonomousReason = "error"
)

// AutonomousLimits are the configured bounds.
type AutonomousLimits struct {
	MaxTurns         int           `json:"max_turns,omitempty"`         // 0 = unlimited
	MaxTokens        int           `json:"max_tokens,omitempty"`        // 0 = unlimited
	MaxTime          time.Duration `json:"max_time,omitempty"`          // 0 = unlimited
	MaxContinuations int           `json:"max_continuations,omitempty"` // 0 = unlimited
}

// AutonomousStatus is the live tracking state.
type AutonomousStatus struct {
	Limits            AutonomousLimits `json:"limits"`
	TurnsUsed         int              `json:"turns_used"`
	TokensUsed        int              `json:"tokens_used"`
	TimeUsedSeconds   float64          `json:"time_used_seconds"`
	ContinuationsUsed int              `json:"continuations_used"`
	Reason            AutonomousReason `json:"reason"`
	LastError         string           `json:"last_error,omitempty"`
	StartedAt         time.Time        `json:"started_at"`
}

// AutonomousBudget tracks a bounded autonomous run.
type AutonomousBudget struct {
	limits  AutonomousLimits
	status  AutonomousStatus
	started time.Time
}

// NewAutonomousBudget starts a bounded run with the given limits.
func NewAutonomousBudget(limits AutonomousLimits) *AutonomousBudget {
	return &AutonomousBudget{
		limits:  limits,
		status:  AutonomousStatus{Limits: limits, Reason: ReasonRunning, StartedAt: time.Now()},
		started: time.Now(),
	}
}

// RecordTurn increments the turn counter and reports the current reason.
func (b *AutonomousBudget) RecordTurn() AutonomousReason {
	b.status.TurnsUsed++
	return b.Check()
}

// RecordToken adds usage and reports the current reason.
func (b *AutonomousBudget) RecordToken(n int) AutonomousReason {
	b.status.TokensUsed += n
	return b.Check()
}

// RecordContinuation increments the continuation counter.
func (b *AutonomousBudget) RecordContinuation() AutonomousReason {
	b.status.ContinuationsUsed++
	return b.Check()
}

// Check recomputes the status from budgets + elapsed time. Once a terminal
// reason is reached it stays terminal (first budget hit wins).
func (b *AutonomousBudget) Check() AutonomousReason {
	if b.status.Reason != ReasonRunning {
		return b.status.Reason
	}
	b.status.TimeUsedSeconds = time.Since(b.started).Seconds()
	switch {
	case b.limits.MaxTurns > 0 && b.status.TurnsUsed >= b.limits.MaxTurns:
		b.status.Reason = ReasonBudgetTurns
	case b.limits.MaxTokens > 0 && b.status.TokensUsed >= b.limits.MaxTokens:
		b.status.Reason = ReasonBudgetTokens
	case b.limits.MaxContinuations > 0 && b.status.ContinuationsUsed >= b.limits.MaxContinuations:
		b.status.Reason = ReasonBudgetConcurr
	case b.limits.MaxTime > 0 && time.Since(b.started) >= b.limits.MaxTime:
		b.status.Reason = ReasonBudgetTime
	}
	return b.status.Reason
}

// MarkGatePassed records that a configured quality gate succeeded.
func (b *AutonomousBudget) MarkGatePassed() {
	if b.status.Reason == ReasonRunning {
		b.status.Reason = ReasonGatePassed
	}
}

// MarkError records a run error.
func (b *AutonomousBudget) MarkError(errMsg string) {
	b.status.Reason = ReasonError
	b.status.LastError = errMsg
}

// Status returns a copy of the live status.
func (b *AutonomousBudget) Status() AutonomousStatus { return b.status }

// Running reports whether the run should continue (not terminal).
func (b *AutonomousBudget) Running() bool { return b.status.Reason == ReasonRunning }

// StopReason returns the terminal reason, or ReasonRunning if still running.
func (b *AutonomousBudget) StopReason() AutonomousReason { return b.status.Reason }
