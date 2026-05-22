package engine

import "github.com/GrayCodeAI/hawk/internal/engine/validation"

type GenValidator = validation.GenValidator
type GenCheck = validation.GenCheck
type GenIssue = validation.GenIssue
type GenValidation = validation.GenValidation
type SchemaValidator = validation.SchemaValidator
type Schema = validation.Schema
type FieldSpec = validation.FieldSpec
type SchemaValidationResult = validation.SchemaValidationResult
type SchemaValidationError = validation.SchemaValidationError
type TestLoop = validation.TestLoop
type TestResult = validation.TestResult
type LintLoop = validation.LintLoop
type LintResult = validation.LintResult

func NewGenValidator() *GenValidator           { return validation.NewGenValidator() }
func NewSchemaValidator() *SchemaValidator     { return validation.NewSchemaValidator() }
func NewTestLoop() *TestLoop                   { return validation.NewTestLoop() }
func NewLintLoop() *LintLoop                   { return validation.NewLintLoop() }
