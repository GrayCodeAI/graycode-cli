// Package safety is the Stage-1 namespace for hallucination guard, output redaction, permissions, risk assessment.
// See ../REFACTOR_PLAN.md.
package safety

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	HallucinationGuard = engine.HallucinationGuard
	GroundingResult    = engine.GroundingResult
	RedactPattern      = engine.RedactPattern
	RedactStats        = engine.RedactStats
	OutputRedactor     = engine.OutputRedactor
	PermissionRequest  = engine.PermissionRequest
	PermissionMemory   = engine.PermissionMemory
	PermissionMode     = engine.PermissionMode
	PermissionEngine   = engine.PermissionEngine
	ProtectedPaths     = engine.ProtectedPaths
	RiskAssessment     = engine.RiskAssessment
	RiskFactor         = engine.RiskFactor
	RiskFactorDef      = engine.RiskFactorDef
	RiskContext        = engine.RiskContext
	RiskAssessor       = engine.RiskAssessor
	AutonomyLevel      = engine.AutonomyLevel
	AutonomyConfig     = engine.AutonomyConfig
)

var (
	NewHallucinationGuard = engine.NewHallucinationGuard
	BuildRejectionMessage = engine.BuildRejectionMessage
	FormatGroundingResult = engine.FormatGroundingResult
	NewOutputRedactor     = engine.NewOutputRedactor
	NewPermissionMemory   = engine.NewPermissionMemory
	NewPermissionEngine   = engine.NewPermissionEngine
	NewProtectedPaths     = engine.NewProtectedPaths
	NewRiskAssessor       = engine.NewRiskAssessor
	GenerateMitigations   = engine.GenerateMitigations
	FormatAssessment      = engine.FormatAssessment
	ShouldProceed         = engine.ShouldProceed
	PresetConfig          = engine.PresetConfig
	ParseAutonomyLevel    = engine.ParseAutonomyLevel
)
