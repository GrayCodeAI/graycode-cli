package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/GrayCodeAI/hawk/internal/engine/branching"
)

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
			ch <- StreamEvent{Type: "content", Content: fmt.Sprintf("\n\nLimit reached: %s", reason)}
			ch <- StreamEvent{Type: "done"}
			return false
		}
	}

	if s.MaxTurns > 0 && turnCount >= s.MaxTurns {
		ch <- StreamEvent{Type: "content", Content: "Turn limit reached — stopping."}
		ch <- StreamEvent{Type: "done"}
		return false
	}

	return true
}
