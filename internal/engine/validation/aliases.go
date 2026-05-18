// Package validation is the Stage-1 namespace for generated-code validation,
// schema validation, test loops, and lint loops. See ../REFACTOR_PLAN.md.
package validation

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	GenValidator           = engine.GenValidator
	GenCheck               = engine.GenCheck
	GenIssue               = engine.GenIssue
	GenValidation          = engine.GenValidation
	SchemaValidator        = engine.SchemaValidator
	Schema                 = engine.Schema
	FieldSpec              = engine.FieldSpec
	SchemaValidationResult = engine.SchemaValidationResult
	SchemaValidationError  = engine.SchemaValidationError
	TestLoop               = engine.TestLoop
	TestResult             = engine.TestResult
	LintLoop               = engine.LintLoop
	LintResult             = engine.LintResult
)

func NewGenValidator() *GenValidator                    { return engine.NewGenValidator() }
func NewSchemaValidator() *SchemaValidator              { return engine.NewSchemaValidator() }
func NewTestLoop() *TestLoop                            { return engine.NewTestLoop() }
func NewLintLoop() *LintLoop                            { return engine.NewLintLoop() }
func ValidateGo(code string) []GenIssue                 { return engine.ValidateGo(code) }
func ValidatePython(code string) []GenIssue             { return engine.ValidatePython(code) }
func ValidateTypeScript(code string) []GenIssue         { return engine.ValidateTypeScript(code) }
func ExtractJSONFromOutput(text string) (string, error) { return engine.ExtractJSONFromOutput(text) }

func ExtractCodeFromOutput(text, lang string) (string, error) {
	return engine.ExtractCodeFromOutput(text, lang)
}
func RepairJSON(broken string) (string, error)   { return engine.RepairJSON(broken) }
func DetectTestCommand(projectDir string) string { return engine.DetectTestCommand(projectDir) }
