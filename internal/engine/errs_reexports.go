package engine

import "github.com/GrayCodeAI/hawk/internal/engine/errs"

type (
	ErrorContext      = errs.ErrorContext
	ErrorHelp         = errs.ErrorHelp
	EnrichedError     = errs.EnrichedError
	ErrorGroup        = errs.ErrorGroup
	ErrorInstance     = errs.ErrorInstance
	ErrorGrouper      = errs.ErrorGrouper
	ErrorPattern      = errs.ErrorPattern
	ErrorPatternDB    = errs.ErrorPatternDB
	ErrorLearner      = errs.ErrorLearner
	ErrorLearnerStats = errs.ErrorLearnerStats
	LearnedPattern    = errs.LearnedPattern
	ErrorRecovery     = errs.ErrorRecovery
	RecoveryStrategy  = errs.RecoveryStrategy
	RecoveryContext   = errs.RecoveryContext
	RecoveryResult    = errs.RecoveryResult
	RecoveryAttempt   = errs.RecoveryAttempt
)

func NewErrorContext() *ErrorContext             { return errs.NewErrorContext() }
func FormatError(enriched *EnrichedError) string { return errs.FormatError(enriched) }
func NewErrorGrouper() *ErrorGrouper             { return errs.NewErrorGrouper() }
func NormalizeError(msg string) string           { return errs.NormalizeError(msg) }
func NewErrorLearner() *ErrorLearner             { return errs.NewErrorLearner() }
func ExtractPattern(errorMsg string) string      { return errs.ExtractPattern(errorMsg) }
func NewErrorPatternDB() *ErrorPatternDB         { return errs.NewErrorPatternDB() }
func NewErrorRecovery() *ErrorRecovery           { return errs.NewErrorRecovery() }
