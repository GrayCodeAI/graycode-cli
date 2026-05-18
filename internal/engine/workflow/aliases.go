// Package workflow is the Stage-1 namespace for workflow + workspace +
// trajectory types in package engine. See ../REFACTOR_PLAN.md.
package workflow

import (
	"context"

	"github.com/GrayCodeAI/eyrie/client"
	"github.com/GrayCodeAI/hawk/internal/engine"
)

// Workflow is a declarative multi-step task definition.
type Workflow = engine.Workflow

// Step is one node in a Workflow.
type Step = engine.WorkflowStep

// Result is the outcome of running a Workflow.
type Result = engine.WorkflowResult

// StepResult is the outcome of a single step.
type StepResult = engine.StepResult

// Engine executes Workflows against a model-call function.
type Engine = engine.WorkflowEngine

// WorkspaceState captures the on-disk state of files at a point in time.
type WorkspaceState = engine.WorkspaceState

// FileState is the per-file slice of WorkspaceState.
type FileState = engine.FileState

// DiffReport is a structured workspace diff.
type DiffReport = engine.WorkspaceDiffReport

// FileDiffReport is the per-file slice of DiffReport.
type FileDiffReport = engine.FileDiffReport

// DiffReporter computes DiffReports between WorkspaceStates.
type DiffReporter = engine.DiffReporter

// TrajectoryRun is one execution of an agent loop.
type TrajectoryRun = engine.TrajectoryRun

// TrajectoryDistiller summarises a slice of runs into a learnt strategy.
type TrajectoryDistiller = engine.TrajectoryDistiller

// TrajectoryEvent is one entry in a TrajectoryInspector.
type TrajectoryEvent = engine.TrajectoryEvent

// TrajectoryInspector records events during an agent loop for offline review.
type TrajectoryInspector = engine.TrajectoryInspector

// NewEngine returns a workflow engine that delegates step execution to the
// provided function (which typically wraps an LLM call).
func NewEngine(executeFn func(ctx context.Context, agent, prompt string) (string, error)) *Engine {
	return engine.NewWorkflowEngine(executeFn)
}

// NewWorkspaceState returns a fresh state snapshot rooted at projectDir.
func NewWorkspaceState(projectDir string) *WorkspaceState {
	return engine.NewWorkspaceState(projectDir)
}

// NewDiffReporter returns a reporter rooted at projectDir.
func NewDiffReporter(projectDir string) *DiffReporter {
	return engine.NewDiffReporter(projectDir)
}

// NewTrajectoryInspector returns an inspector scoped to sessionID.
func NewTrajectoryInspector(sessionID string) *TrajectoryInspector {
	return engine.NewTrajectoryInspector(sessionID)
}

// SubstituteVars expands template placeholders against vars.
func SubstituteVars(template string, vars map[string]string) string {
	return engine.SubstituteVars(template, vars)
}

// EvalCondition evaluates a workflow guard expression.
func EvalCondition(condition string, vars map[string]string) bool {
	return engine.EvalCondition(condition, vars)
}

// ValidateWorkflow returns a slice of human-readable validation errors.
func ValidateWorkflow(wf *Workflow) []string {
	return engine.ValidateWorkflow(wf)
}

// BuiltinWorkflows is the set of workflows shipped with hawk.
func BuiltinWorkflows() map[string]*Workflow {
	return engine.BuiltinWorkflows()
}

// FormatAsMarkdown renders a DiffReport as Markdown.
func FormatAsMarkdown(report *DiffReport) string {
	return engine.FormatAsMarkdown(report)
}

// FormatAsTerminal renders a DiffReport for the terminal.
func FormatAsTerminal(report *DiffReport) string {
	return engine.FormatAsTerminal(report)
}

// FormatForCommit renders a DiffReport as a commit-message body.
func FormatForCommit(report *DiffReport) string {
	return engine.FormatForCommit(report)
}

// CompareReports diffs two DiffReports.
func CompareReports(before, after *DiffReport) string {
	return engine.CompareReports(before, after)
}

// SummarizeTrajectory produces a one-line summary of a message run.
func SummarizeTrajectory(messages []client.EyrieMessage) string {
	return engine.SummarizeTrajectory(messages)
}
