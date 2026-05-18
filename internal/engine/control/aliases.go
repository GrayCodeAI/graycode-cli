// Package control is the Stage-1 namespace for engine control-flow safety
// types — loop detection, stall detection, backtracking. See ../REFACTOR_PLAN.md.
package control

import "github.com/GrayCodeAI/hawk/internal/engine"

// LoopDetector watches for repeated tool-call patterns indicating the agent
// is stuck in a doom loop.
type LoopDetector = engine.LoopDetector

// DoomLoopThreshold is the number of identical recent actions that flips a
// LoopDetector into "stuck" state.
const DoomLoopThreshold = engine.DoomLoopThreshold

// NewLoopDetector returns a detector with the given sliding-window size and
// max-repeats threshold.
func NewLoopDetector(windowSize, maxRepeats int) *LoopDetector {
	return engine.NewLoopDetector(windowSize, maxRepeats)
}

// StallEntry is one observed action in the stall window.
type StallEntry = engine.StallEntry

// StallResult is the verdict of a single stall check.
type StallResult = engine.StallResult

// StallDetector flags long stretches of no observable progress.
type StallDetector = engine.StallDetector

// NewStallDetector returns a detector with default thresholds.
func NewStallDetector() *StallDetector {
	return engine.NewStallDetector()
}

// DecisionPoint is a snapshot the agent can return to.
type DecisionPoint = engine.DecisionPoint

// BacktrackEngine manages decision points and the rollback path.
type BacktrackEngine = engine.BacktrackEngine

// NewBacktrackEngine returns a fresh backtrack engine.
func NewBacktrackEngine() *BacktrackEngine {
	return engine.NewBacktrackEngine()
}
