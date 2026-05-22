package engine

import "github.com/GrayCodeAI/hawk/internal/engine/errs"

type ErrorContext = errs.ErrorContext
type ErrorHelp = errs.ErrorHelp
type EnrichedError = errs.EnrichedError
type ErrorGroup = errs.ErrorGroup
type ErrorInstance = errs.ErrorInstance
type ErrorGrouper = errs.ErrorGrouper
type ErrorPattern = errs.ErrorPattern
type ErrorPatternDB = errs.ErrorPatternDB
type ErrorLearner = errs.ErrorLearner
type ErrorLearnerStats = errs.ErrorLearnerStats
type LearnedPattern = errs.LearnedPattern
type ErrorRecovery = errs.ErrorRecovery
type RecoveryStrategy = errs.RecoveryStrategy
type RecoveryContext = errs.RecoveryContext
type RecoveryResult = errs.RecoveryResult
type RecoveryAttempt = errs.RecoveryAttempt

func NewErrorContext() *ErrorContext                          { return errs.NewErrorContext() }
func FormatError(enriched *EnrichedError) string              { return errs.FormatError(enriched) }
func NewErrorGrouper() *ErrorGrouper                          { return errs.NewErrorGrouper() }
func NormalizeError(msg string) string                        { return errs.NormalizeError(msg) }
func NewErrorLearner() *ErrorLearner                          { return errs.NewErrorLearner() }
func ExtractPattern(errorMsg string) string                   { return errs.ExtractPattern(errorMsg) }
func NewErrorPatternDB() *ErrorPatternDB                      { return errs.NewErrorPatternDB() }
func NewErrorRecovery() *ErrorRecovery                        { return errs.NewErrorRecovery() }
