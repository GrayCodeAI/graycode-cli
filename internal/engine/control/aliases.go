// Package control provides engine control-flow safety types — loop
// detection, stall detection, and backtracking.
//
// Public types: LoopDetector, StallEntry, StallResult, StallDetector,
// DecisionPoint, BacktrackEngine.
//
// Public functions: NewLoopDetector, NewStallDetector, NewBacktrackEngine.
// Public constants: DoomLoopThreshold.
package control
