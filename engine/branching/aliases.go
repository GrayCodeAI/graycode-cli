// Package branching is the Stage-1 namespace for branching strategies, cascade, council, shadow, snowball.
// See ../REFACTOR_PLAN.md.
package branching

import "github.com/GrayCodeAI/hawk/engine"

type BranchMessage = engine.BranchMessage
type ConversationBranch = engine.ConversationBranch
type BranchManager = engine.BranchManager
type CascadeRouter = engine.CascadeRouter
type RoutingDecision = engine.RoutingDecision
type ModelTier = engine.ModelTier
type CouncilConfig = engine.CouncilConfig
type CouncilResponse = engine.CouncilResponse
type CouncilRanking = engine.CouncilRanking
type CouncilResult = engine.CouncilResult
type ShadowWorkspace = engine.ShadowWorkspace
type SnowballDetector = engine.SnowballDetector

var NewBranchManager = engine.NewBranchManager
var NewCascadeRouter = engine.NewCascadeRouter
var RunCouncil = engine.RunCouncil
var DefaultCouncilModels = engine.DefaultCouncilModels
var NewShadowWorkspace = engine.NewShadowWorkspace
var NewSnowballDetector = engine.NewSnowballDetector
