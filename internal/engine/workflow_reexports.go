package engine

import (
	"context"

	"github.com/GrayCodeAI/hawk/internal/engine/workflow"
)

// Types from workflow sub-package.

type (
	Workflow            = workflow.Workflow
	WorkflowStep        = workflow.WorkflowStep
	WorkflowResult      = workflow.WorkflowResult
	StepResult          = workflow.StepResult
	WorkflowEngine      = workflow.WorkflowEngine
	WorkspaceState      = workflow.WorkspaceState
	FileState           = workflow.FileState
	WorkspaceDiffReport = workflow.WorkspaceDiffReport
	FileDiffReport      = workflow.FileDiffReport
	DiffReporter        = workflow.DiffReporter
	TrajectoryEvent     = workflow.TrajectoryEvent
	TrajectoryInspector = workflow.TrajectoryInspector
)

// Short-name aliases provided by the workflow sub-package.

type (
	Step   = workflow.WorkflowStep
	Result = workflow.WorkflowResult
	Engine = workflow.WorkflowEngine
)

// Functions.

var (
	NewWorkflowEngine      = workflow.NewWorkflowEngine
	NewWorkspaceState      = workflow.NewWorkspaceState
	NewDiffReporter        = workflow.NewDiffReporter
	NewTrajectoryInspector = workflow.NewTrajectoryInspector
	SubstituteVars         = workflow.SubstituteVars
	EvalCondition          = workflow.EvalCondition
	ValidateWorkflow       = workflow.ValidateWorkflow
	BuiltinWorkflows       = workflow.BuiltinWorkflows
	FormatAsMarkdown       = workflow.FormatAsMarkdown
	FormatAsTerminal       = workflow.FormatAsTerminal
	FormatForCommit        = workflow.FormatForCommit
	CompareReports         = workflow.CompareReports
)

// NewEngine delegates to workflow.NewEngine.
func NewEngine(executeFn func(ctx context.Context, agent, prompt string) (string, error)) *Engine {
	return workflow.NewEngine(executeFn)
}

// TrajectoryRun, TrajectoryDistiller, NewTrajectoryDistiller, SummarizeTrajectory
// are defined directly in package engine (trajectory.go).
