package engine

import (
	"fmt"
	"strings"

	"github.com/hawk/eyrie/client"
)

// DecisionPoint captures a point in the conversation where the agent made a
// choice that can potentially be rolled back if it leads to failure.
type DecisionPoint struct {
	TurnIndex    int
	Description  string                // what was decided
	Alternatives []string              // other options available
	Outcome      string                // "success", "failure", or "" (pending)
	Messages     []client.EyrieMessage // conversation state at this point
}

// BacktrackEngine records decision points during agent execution and provides
// the ability to identify the most recent failure and generate a retry prompt
// with alternative approaches.
type BacktrackEngine struct {
	points    []DecisionPoint
	maxPoints int
}

// NewBacktrackEngine creates a new backtrack engine that retains at most 50
// decision points.
func NewBacktrackEngine() *BacktrackEngine {
	return &BacktrackEngine{
		maxPoints: 50,
	}
}

// RecordDecision saves a decision point with the current conversation state.
// If the number of recorded points exceeds the maximum, the oldest point is
// removed.
func (be *BacktrackEngine) RecordDecision(turnIdx int, desc string, alternatives []string, msgs []client.EyrieMessage) {
	// Copy messages to avoid mutation from caller
	snapshot := make([]client.EyrieMessage, len(msgs))
	copy(snapshot, msgs)

	// Copy alternatives
	alts := make([]string, len(alternatives))
	copy(alts, alternatives)

	dp := DecisionPoint{
		TurnIndex:    turnIdx,
		Description:  desc,
		Alternatives: alts,
		Messages:     snapshot,
	}

	be.points = append(be.points, dp)

	// Evict oldest if over limit
	if len(be.points) > be.maxPoints {
		be.points = be.points[1:]
	}
}

// MarkOutcome sets the outcome ("success" or "failure") for the decision at
// the given turn index. If multiple decisions exist at the same turn index,
// the most recent one is updated.
func (be *BacktrackEngine) MarkOutcome(turnIdx int, outcome string) {
	// Walk backwards to find the most recent decision at this turn
	for i := len(be.points) - 1; i >= 0; i-- {
		if be.points[i].TurnIndex == turnIdx {
			be.points[i].Outcome = outcome
			return
		}
	}
}

// FindBacktrackPoint returns the most recent failed decision point that has
// alternative approaches available, or nil if none exists.
func (be *BacktrackEngine) FindBacktrackPoint() *DecisionPoint {
	for i := len(be.points) - 1; i >= 0; i-- {
		dp := &be.points[i]
		if dp.Outcome == "failure" && len(dp.Alternatives) > 0 {
			return dp
		}
	}
	return nil
}

// GenerateRetryPrompt builds a prompt that tells the agent what failed and
// suggests alternative approaches.
func (be *BacktrackEngine) GenerateRetryPrompt(dp *DecisionPoint) string {
	if dp == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Previous approach failed: %s.", dp.Description))
	b.WriteString(fmt.Sprintf(" The outcome was: %s.", dp.Outcome))

	if len(dp.Alternatives) > 0 {
		b.WriteString("\nAlternative approaches to try: ")
		b.WriteString(strings.Join(dp.Alternatives, "; "))
		b.WriteString(".")
	}

	b.WriteString("\nPlease try a different approach.")

	return b.String()
}

// RestoreState returns the conversation messages captured at the given decision
// point, representing the state just before (not including) the failed
// decision. This allows the agent to retry from that point.
func (be *BacktrackEngine) RestoreState(dp *DecisionPoint) []client.EyrieMessage {
	if dp == nil {
		return nil
	}
	// Return a copy to prevent mutation
	restored := make([]client.EyrieMessage, len(dp.Messages))
	copy(restored, dp.Messages)
	return restored
}

// Size returns the number of recorded decision points.
func (be *BacktrackEngine) Size() int {
	return len(be.points)
}
