// This file re-exports symbols from the planning sub-package so that existing
// callers of engine.ExecutionPlan, engine.NewExecutionPlanner, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/planning"

type (
	ExecutionPlan    = planning.ExecutionPlan
	ExecutionStep    = planning.ExecutionStep
	PlannedCall      = planning.PlannedCall
	ExecutionPlanner = planning.ExecutionPlanner
	Task             = planning.Task
	TaskPlan         = planning.TaskPlan
	TaskDecomposer   = planning.TaskDecomposer
	Subtask          = planning.Subtask
	PlanState        = planning.PlanState
	GoalStatus       = planning.GoalStatus
	Goal             = planning.Goal
	GoalEvent        = planning.GoalEvent
	GoalTracker      = planning.GoalTracker
	GoalOption       = planning.GoalOption
	SuggestedTask    = planning.SuggestedTask
	TaskQueue        = planning.TaskQueue
	ActionRequired   = planning.ActionRequired
	FormField        = planning.FormField
	FormResponse     = planning.FormResponse
	ActionManager    = planning.ActionManager
)

var (
	NewExecutionPlanner = planning.NewExecutionPlanner
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
