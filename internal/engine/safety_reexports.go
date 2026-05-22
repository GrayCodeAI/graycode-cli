package engine

import "github.com/GrayCodeAI/hawk/internal/engine/safety"

type (
	HallucinationGuard = safety.HallucinationGuard
	GroundingResult    = safety.GroundingResult
	RedactPattern      = safety.RedactPattern
	RedactStats        = safety.RedactStats
	OutputRedactor     = safety.OutputRedactor
	PermissionRequest  = safety.PermissionRequest
	PermissionMemory   = safety.PermissionMemory
	PermissionMode     = safety.PermissionMode
	PermissionEngine   = safety.PermissionEngine
	ProtectedPaths     = safety.ProtectedPaths
	RiskAssessment     = safety.RiskAssessment
	RiskFactor         = safety.RiskFactor
	RiskFactorDef      = safety.RiskFactorDef
	RiskContext        = safety.RiskContext
	RiskAssessor       = safety.RiskAssessor
	AutonomyLevel      = safety.AutonomyLevel
	AutonomyConfig     = safety.AutonomyConfig
	ToolCallInfo       = safety.ToolCallInfo
)

const (
	AutonomySupervised              = safety.AutonomySupervised
	AutonomyBasic                   = safety.AutonomyBasic
	AutonomySemi                    = safety.AutonomySemi
	AutonomyFull                    = safety.AutonomyFull
	AutonomyYOLO                    = safety.AutonomyYOLO
	PermissionModeDefault           = safety.PermissionModeDefault
	PermissionModeAcceptEdits       = safety.PermissionModeAcceptEdits
	PermissionModeBypassPermissions = safety.PermissionModeBypassPermissions
	PermissionModeDontAsk           = safety.PermissionModeDontAsk
	PermissionModePlan              = safety.PermissionModePlan
)

var (
	NewHallucinationGuard = safety.NewHallucinationGuard
	BuildRejectionMessage = safety.BuildRejectionMessage
	FormatGroundingResult = safety.FormatGroundingResult
	NewOutputRedactor     = safety.NewOutputRedactor
	NewPermissionMemory   = safety.NewPermissionMemory
	NewPermissionEngine   = safety.NewPermissionEngine
	NewProtectedPaths     = safety.NewProtectedPaths
	NewRiskAssessor       = safety.NewRiskAssessor
	GenerateMitigations   = safety.GenerateMitigations
	FormatAssessment      = safety.FormatAssessment
	ShouldProceed         = safety.ShouldProceed
	PresetConfig          = safety.PresetConfig
	ParseAutonomyLevel    = safety.ParseAutonomyLevel
	ToolSummary          = safety.ToolSummary
	ToolNeedsPermission  = safety.ToolNeedsPermission
)
