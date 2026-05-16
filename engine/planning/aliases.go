// Package planning is the Stage-1 namespace for task planning, decomposition, goals, and suggested tasks.
// See ../REFACTOR_PLAN.md.
package planning

import "github.com/GrayCodeAI/hawk/engine"

type (
	ExecutionPlan    = engine.ExecutionPlan
	ExecutionStep    = engine.ExecutionStep
	PlannedCall      = engine.PlannedCall
	ExecutionPlanner = engine.ExecutionPlanner
	Task             = engine.Task
	TaskPlan         = engine.TaskPlan
	TaskDecomposer   = engine.TaskDecomposer
	Subtask          = engine.Subtask
	PlanState        = engine.PlanState
	GoalStatus       = engine.GoalStatus
	Goal             = engine.Goal
	GoalEvent        = engine.GoalEvent
	GoalTracker      = engine.GoalTracker
	GoalOption       = engine.GoalOption
	SuggestedTask    = engine.SuggestedTask
	TaskQueue        = engine.TaskQueue
	ActionRequired   = engine.ActionRequired
	FormField        = engine.FormField
	FormResponse     = engine.FormResponse
	ActionManager    = engine.ActionManager
)

var (
	NewExecutionPlanner = engine.NewExecutionPlanner
	NewTaskDecomposer   = engine.NewTaskDecomposer
	NewPlanState        = engine.NewPlanState
	DecomposePrompt     = engine.DecomposePrompt
	ParseSubtasks       = engine.ParseSubtasks
	WithPriority        = engine.WithPriority
	WithBudget          = engine.WithBudget
	WithDependencies    = engine.WithDependencies
	WithTags            = engine.WithTags
	NewGoalTracker      = engine.NewGoalTracker
	NewTaskQueue        = engine.NewTaskQueue
	ScanGitTasks        = engine.ScanGitTasks
	ScanTODOs           = engine.ScanTODOs
	ScanTestFailures    = engine.ScanTestFailures
	FormatTasks         = engine.FormatTasks
	NewActionManager    = engine.NewActionManager
	Validate            = engine.Validate
	BuildFormPrompt     = engine.BuildFormPrompt
	FormatResponse      = engine.FormatResponse
)
