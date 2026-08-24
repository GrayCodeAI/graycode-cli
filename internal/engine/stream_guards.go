package engine

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/GrayCodeAI/hawk/internal/engine/branching"
)

// gracefulExhaustionEnv opts the guard path into exhaustion synthesis. Off by
// default: synthesis is a blocking provider call and the default loop must
// stop immediately when limits are hit.
const gracefulExhaustionEnv = "HAWK_GRACEFUL_EXHAUSTION"

// checkGuardConditions runs all pre-turn guard checks.
// Returns false when the loop should stop (abort conditions met).
// On first-stage loop detection, injects a break-loop message and continues.
func (s *Session) checkGuardConditions(ctx context.Context, ch chan<- StreamEvent, turnCount int, snowball *branching.SnowballDetector, loopDet *LoopDetector) bool {
	if ctx.Err() != nil {
		msg := "Request cancelled."
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			msg = "Time budget exhausted."
		}
		ch <- StreamEvent{Type: "content", Content: "\n\n" + msg}
		ch <- StreamEvent{Type: "done"}
		return false
	}

	if snowball.ShouldAbort() {
		ch <- StreamEvent{Type: "content", Content: "\n\n" + snowball.Summary()}
		ch <- StreamEvent{Type: "done"}
		return false
	}

	if loopDet.IsDoomLoop() {
		ch <- StreamEvent{Type: "content", Content: "\n\n" + "STOP:" + " " + loopDet.DoomLoopWarning()}
		ch <- StreamEvent{Type: "done"}
		return false
	}
	if loopDet.IsLooping() && !loopDet.Escalated() {
		loopDet.MarkEscalated()
		s.AddAssistant(loopDet.LoopWarning())
		s.AddUser("You are stuck in a loop. Try a completely different approach. If you cannot make progress, explain what's blocking you.")
	}

	if s.LifecycleSvc().Limits() != nil {
		if exceeded, reason := s.LifecycleSvc().Limits().IsExceeded(); exceeded {
			s.emitExhaustion(ctx, ch, reason)
			return false
		}
	}
	if allowed, reason := s.tokUsageCanProceed(); !allowed {
		s.emitExhaustion(ctx, ch, reason)
		return false
	}

	if s.LifecycleSvc() != nil && s.LifecycleSvc().Limits().MaxTurns() > 0 && turnCount >= s.LifecycleSvc().Limits().MaxTurns() {
		s.emitExhaustion(ctx, ch, "turn limit reached")
		return false
	}

	return true
}

// emitExhaustion stops the loop with a graceful completion: one final
// tools-disabled LLM call synthesizes a summary of the work (herm's graceful
// exhaustion). Falls back to a static "limit reached" message when synthesis is
// unavailable or fails.
//
// Synthesis performs a blocking provider call, so it is opt-in via
// HAWK_GRACEFUL_EXHAUSTION=1: the guard path must stay fast and non-blocking
// by default (a stalled guard delays every terminal event downstream).
func (s *Session) emitExhaustion(ctx context.Context, ch chan<- StreamEvent, reason string) {
	if gracefulExhaustionEnabled() {
		if synth := s.SynthesisForExhaustion(ctx, reason); synth != "" {
			ch <- StreamEvent{Type: "content", Content: "\n\n" + synth}
			ch <- StreamEvent{Type: "done"}
			return
		}
	}
	ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n\nLimit reached: %s", reason)}
	ch <- StreamEvent{Type: "done"}
}

// gracefulExhaustionEnabled reports whether opt-in exhaustion synthesis is on.
func gracefulExhaustionEnabled() bool {
	switch os.Getenv(gracefulExhaustionEnv) {
	case "1", "true", "TRUE", "True":
		return true
	default:
		return false
	}
}
