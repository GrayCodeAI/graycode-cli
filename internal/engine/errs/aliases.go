// Package errs is the Stage-1 namespace for error context enrichment,
// grouping, learning, patterns, and recovery. See ../REFACTOR_PLAN.md.
//
// Named "errs" (not "error") to avoid shadowing the builtin error type.
package errs

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	ErrorContext      = engine.ErrorContext
	ErrorHelp         = engine.ErrorHelp
	EnrichedError     = engine.EnrichedError
	ErrorInstance     = engine.ErrorInstance
	ErrorGroup        = engine.ErrorGroup
	ErrorGrouper      = engine.ErrorGrouper
	LearnedPattern    = engine.LearnedPattern
	ErrorLearnerStats = engine.ErrorLearnerStats
	ErrorLearner      = engine.ErrorLearner
	ErrorPattern      = engine.ErrorPattern
	ErrorPatternDB    = engine.ErrorPatternDB
	ErrorRecovery     = engine.ErrorRecovery
	RecoveryStrategy  = engine.RecoveryStrategy
	RecoveryContext   = engine.RecoveryContext
	RecoveryResult    = engine.RecoveryResult
	RecoveryAttempt   = engine.RecoveryAttempt
)

func NewErrorContext() *ErrorContext      { return engine.NewErrorContext() }
func NewErrorGrouper() *ErrorGrouper      { return engine.NewErrorGrouper() }
func NewErrorLearner() *ErrorLearner      { return engine.NewErrorLearner() }
func NewErrorPatternDB() *ErrorPatternDB  { return engine.NewErrorPatternDB() }
func NewErrorRecovery() *ErrorRecovery    { return engine.NewErrorRecovery() }
func FormatError(e *EnrichedError) string { return engine.FormatError(e) }
func NormalizeError(msg string) string    { return engine.NormalizeError(msg) }
func ExtractPattern(msg string) string    { return engine.ExtractPattern(msg) }
