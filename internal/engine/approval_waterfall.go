package engine

import (
	"context"
	"fmt"
)

// ApprovalDecider is one link in the approval waterfall. It returns the response and
// true when it has an authoritative answer, or (default, false) to abstain and pass
// control to the next decider.
type ApprovalDecider func(ctx context.Context, req ApprovalRequest) (ApprovalResponse, bool)

// ApprovalWaterfall answers approval requests through an ordered chain of deciders.
// It is the deepseek-harness approval/request waterfall port: the first decider
// that answers wins; a decider that abstains delegates to the next. If every
// decider abstains — including the empty chain — the gate is fail-closed and the
// action is denied. Deciders run synchronously in registration order.
type ApprovalWaterfall struct {
	deciders []ApprovalDecider
}

// NewApprovalWaterfall constructs an empty waterfall.
func NewApprovalWaterfall() *ApprovalWaterfall {
	return &ApprovalWaterfall{}
}

// Add appends a decider and returns a disposer that removes exactly it.
func (w *ApprovalWaterfall) Add(fn ApprovalDecider) func() {
	if w == nil || fn == nil {
		return func() {}
	}
	w.deciders = append(w.deciders, fn)
	idx := len(w.deciders) - 1
	return func() { w.remove(idx) }
}

func (w *ApprovalWaterfall) remove(idx int) {
	if w == nil || idx < 0 || idx >= len(w.deciders) {
		return
	}
	w.deciders = append(w.deciders[:idx], w.deciders[idx+1:]...)
}

// Decide runs the waterfall. It returns (ApprovalReject, denyMessage) when no
// decider answers, implementing the fail-closed guarantee.
func (w *ApprovalWaterfall) Decide(ctx context.Context, req ApprovalRequest) (ApprovalResponse, string) {
	if w != nil {
		for _, fn := range w.deciders {
			if resp, ok := fn(ctx, req); ok {
				if resp == ApprovalReject {
					return resp, fmt.Sprintf("approval denied for %s (%s)", req.Category, req.ToolName)
				}
				return resp, ""
			}
		}
	}
	return ApprovalReject, fmt.Sprintf("approval required for %s (%s) but no answerer is configured", req.Category, req.ToolName)
}
