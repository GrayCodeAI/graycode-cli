package engine

import "github.com/GrayCodeAI/graycode-cli/internal/engine/validation"

type (
	GenValidator           = validation.GenValidator
	GenCheck               = validation.GenCheck
	GenIssue               = validation.GenIssue
	GenValidation          = validation.GenValidation
	SchemaValidator        = validation.SchemaValidator
	Schema                 = validation.Schema
	FieldSpec              = validation.FieldSpec
	SchemaValidationResult = validation.SchemaValidationResult
	SchemaValidationError  = validation.SchemaValidationError
	TestLoop               = validation.TestLoop
	TestResult             = validation.TestResult
	LintLoop               = validation.LintLoop
	LintResult             = validation.LintResult
)

func NewGenValidator() *GenValidator       { return validation.NewGenValidator() }
func NewSchemaValidator() *SchemaValidator { return validation.NewSchemaValidator() }
func NewTestLoop() *TestLoop               { return validation.NewTestLoop() }
func NewLintLoop() *LintLoop               { return validation.NewLintLoop() }
