// Package safety is the Stage-1 namespace for hallucination guard, output redaction, permissions, risk assessment.
// See ../REFACTOR_PLAN.md.
package safety

import "github.com/GrayCodeAI/hawk/engine"

type HallucinationGuard = engine.HallucinationGuard
type GroundingResult = engine.GroundingResult
type RedactPattern = engine.RedactPattern
type RedactStats = engine.RedactStats
type OutputRedactor = engine.OutputRedactor
type PermissionRequest = engine.PermissionRequest
type PermissionMemory = engine.PermissionMemory
type PermissionMode = engine.PermissionMode
type PermissionEngine = engine.PermissionEngine
type ProtectedPaths = engine.ProtectedPaths
type RiskAssessment = engine.RiskAssessment
type RiskFactor = engine.RiskFactor
type RiskFactorDef = engine.RiskFactorDef
type RiskContext = engine.RiskContext
type RiskAssessor = engine.RiskAssessor
type AutonomyLevel = engine.AutonomyLevel
type AutonomyConfig = engine.AutonomyConfig

var NewHallucinationGuard = engine.NewHallucinationGuard
var BuildRejectionMessage = engine.BuildRejectionMessage
var FormatGroundingResult = engine.FormatGroundingResult
var NewOutputRedactor = engine.NewOutputRedactor
var NewPermissionMemory = engine.NewPermissionMemory
var NewPermissionEngine = engine.NewPermissionEngine
var NewProtectedPaths = engine.NewProtectedPaths
var NewRiskAssessor = engine.NewRiskAssessor
var GenerateMitigations = engine.GenerateMitigations
var FormatAssessment = engine.FormatAssessment
var ShouldProceed = engine.ShouldProceed
var PresetConfig = engine.PresetConfig
var ParseAutonomyLevel = engine.ParseAutonomyLevel
