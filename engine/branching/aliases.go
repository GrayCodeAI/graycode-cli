// Package branching is the Stage-1 namespace for branching strategies, cascade, council, shadow, snowball.
// See ../REFACTOR_PLAN.md.
package branching

import "github.com/GrayCodeAI/hawk/engine"

type (
	BranchMessage      = engine.BranchMessage
	ConversationBranch = engine.ConversationBranch
	BranchManager      = engine.BranchManager
	CascadeRouter      = engine.CascadeRouter
	RoutingDecision    = engine.RoutingDecision
	ModelTier          = engine.ModelTier
	CouncilConfig      = engine.CouncilConfig
	CouncilResponse    = engine.CouncilResponse
	CouncilRanking     = engine.CouncilRanking
	CouncilResult      = engine.CouncilResult
	ShadowWorkspace    = engine.ShadowWorkspace
	SnowballDetector   = engine.SnowballDetector
)

var (
	NewBranchManager     = engine.NewBranchManager
	NewCascadeRouter     = engine.NewCascadeRouter
	RunCouncil           = engine.RunCouncil
	DefaultCouncilModels = engine.DefaultCouncilModels
	NewShadowWorkspace   = engine.NewShadowWorkspace
	NewSnowballDetector  = engine.NewSnowballDetector
)
