package engine

import "github.com/GrayCodeAI/hawk/internal/engine/control"

type (
	LoopDetector    = control.LoopDetector
	StallEntry      = control.StallEntry
	StallResult     = control.StallResult
	StallDetector   = control.StallDetector
	DecisionPoint   = control.DecisionPoint
	BacktrackEngine = control.BacktrackEngine
)

const DoomLoopThreshold = control.DoomLoopThreshold

var (
	NewLoopDetector    = control.NewLoopDetector
	NewStallDetector   = control.NewStallDetector
	NewBacktrackEngine = control.NewBacktrackEngine
)
