package engine

import "github.com/GrayCodeAI/hawk/internal/engine/control"

type LoopDetector = control.LoopDetector
type StallEntry = control.StallEntry
type StallResult = control.StallResult
type StallDetector = control.StallDetector
type DecisionPoint = control.DecisionPoint
type BacktrackEngine = control.BacktrackEngine

const DoomLoopThreshold = control.DoomLoopThreshold

var NewLoopDetector = control.NewLoopDetector
var NewStallDetector = control.NewStallDetector
var NewBacktrackEngine = control.NewBacktrackEngine
