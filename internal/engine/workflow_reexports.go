package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/engine/workflow"
)

// Types from workflow sub-package.

type Workflow = workflow.Workflow
type WorkflowStep = workflow.WorkflowStep
type WorkflowResult = workflow.WorkflowResult
type StepResult = workflow.StepResult
type WorkflowEngine = workflow.WorkflowEngine
type WorkspaceState = workflow.WorkspaceState
type FileState = workflow.FileState
type WorkspaceDiffReport = workflow.WorkspaceDiffReport
type FileDiffReport = workflow.FileDiffReport
type DiffReporter = workflow.DiffReporter
type TrajectoryEvent = workflow.TrajectoryEvent
type TrajectoryInspector = workflow.TrajectoryInspector

// Short-name aliases provided by the workflow sub-package.

type Step = workflow.WorkflowStep
type Result = workflow.WorkflowResult
type Engine = workflow.WorkflowEngine

// Functions.

var NewWorkflowEngine = workflow.NewWorkflowEngine
var NewWorkspaceState = workflow.NewWorkspaceState
var NewDiffReporter = workflow.NewDiffReporter
var NewTrajectoryInspector = workflow.NewTrajectoryInspector
var SubstituteVars = workflow.SubstituteVars
var EvalCondition = workflow.EvalCondition
var ValidateWorkflow = workflow.ValidateWorkflow
var BuiltinWorkflows = workflow.BuiltinWorkflows
var FormatAsMarkdown = workflow.FormatAsMarkdown
var FormatAsTerminal = workflow.FormatAsTerminal
var FormatForCommit = workflow.FormatForCommit
var CompareReports = workflow.CompareReports

// NewEngine delegates to workflow.NewEngine.
func NewEngine(executeFn func(ctx context.Context, agent, prompt string) (string, error)) *Engine {
	return workflow.NewEngine(executeFn)
}

// TrajectoryRun, TrajectoryDistiller, NewTrajectoryDistiller, SummarizeTrajectory
// are defined directly in package engine (trajectory.go).
