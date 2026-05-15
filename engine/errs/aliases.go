// Package errs is the Stage-1 namespace for error context enrichment,
// grouping, learning, patterns, and recovery. See ../REFACTOR_PLAN.md.
//
// Named "errs" (not "error") to avoid shadowing the builtin error type.
package errs

import "github.com/GrayCodeAI/hawk/engine"

type ErrorContext = engine.ErrorContext
type ErrorHelp = engine.ErrorHelp
type EnrichedError = engine.EnrichedError
type ErrorInstance = engine.ErrorInstance
type ErrorGroup = engine.ErrorGroup
type ErrorGrouper = engine.ErrorGrouper
type LearnedPattern = engine.LearnedPattern
type ErrorLearnerStats = engine.ErrorLearnerStats
type ErrorLearner = engine.ErrorLearner
type ErrorPattern = engine.ErrorPattern
type ErrorPatternDB = engine.ErrorPatternDB
type ErrorRecovery = engine.ErrorRecovery
type RecoveryStrategy = engine.RecoveryStrategy
type RecoveryContext = engine.RecoveryContext
type RecoveryResult = engine.RecoveryResult
type RecoveryAttempt = engine.RecoveryAttempt

func NewErrorContext() *ErrorContext       { return engine.NewErrorContext() }
func NewErrorGrouper() *ErrorGrouper       { return engine.NewErrorGrouper() }
func NewErrorLearner() *ErrorLearner       { return engine.NewErrorLearner() }
func NewErrorPatternDB() *ErrorPatternDB   { return engine.NewErrorPatternDB() }
func NewErrorRecovery() *ErrorRecovery     { return engine.NewErrorRecovery() }
func FormatError(e *EnrichedError) string  { return engine.FormatError(e) }
func NormalizeError(msg string) string     { return engine.NormalizeError(msg) }
func ExtractPattern(msg string) string     { return engine.ExtractPattern(msg) }
