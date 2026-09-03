// This file re-exports symbols from the planning sub-package so that existing
// callers of engine.ExecutionPlan, engine.NewExecutionPlanner, etc. keep compiling
// during the Stage 2 migration. See docs/plans/engine-refactor-plan.md.
package engine

import "github.com/GrayCodeAI/graycode-cli/internal/engine/planning"

type (
	ExecutionPlan     = planning.ExecutionPlan
	ExecutionStep     = planning.ExecutionStep
	PlannedCall       = planning.PlannedCall
	ExecutionPlanner  = planning.ExecutionPlanner
	BlastRadius       = planning.BlastRadius
	BlastRadiusReport = planning.BlastRadiusReport
	Task              = planning.Task
	TaskPlan          = planning.TaskPlan
	TaskDecomposer    = planning.TaskDecomposer
	Subtask           = planning.Subtask
	PlanState         = planning.PlanState
	GoalStatus        = planning.GoalStatus
	Goal              = planning.Goal
	GoalEvent         = planning.GoalEvent
	GoalTracker       = planning.GoalTracker
	GoalOption        = planning.GoalOption
	SuggestedTask     = planning.SuggestedTask
	TaskQueue         = planning.TaskQueue
	ActionRequired    = planning.ActionRequired
	FormField         = planning.FormField
	FormResponse      = planning.FormResponse
	ActionManager     = planning.ActionManager
)

var (
	NewExecutionPlanner = planning.NewExecutionPlanner
	EstimateBlastRadius = planning.EstimateBlastRadius
	NewTaskDecomposer   = planning.NewTaskDecomposer
	NewPlanState        = planning.NewPlanState
	DecomposePrompt     = planning.DecomposePrompt
	ParseSubtasks       = planning.ParseSubtasks
	WithPriority        = planning.WithPriority
	WithBudget          = planning.WithBudget
	WithDependencies    = planning.WithDependencies
	WithTags            = planning.WithTags
	NewGoalTracker      = planning.NewGoalTracker
	NewTaskQueue        = planning.NewTaskQueue
	ScanGitTasks        = planning.ScanGitTasks
	ScanTODOs           = planning.ScanTODOs
	ScanTestFailures    = planning.ScanTestFailures
	FormatTasks         = planning.FormatTasks
	NewActionManager    = planning.NewActionManager
	Validate            = planning.Validate
	BuildFormPrompt     = planning.BuildFormPrompt
	FormatResponse      = planning.FormatResponse
)

// Blast radius constants re-exported from planning package.
const (
	RadiusSmall  = planning.RadiusSmall
	RadiusMedium = planning.RadiusMedium
	RadiusLarge  = planning.RadiusLarge
	RadiusHuge   = planning.RadiusHuge
)
