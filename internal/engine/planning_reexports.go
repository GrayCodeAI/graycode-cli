// This file re-exports symbols from the planning sub-package so that existing
// callers of engine.ExecutionPlan, engine.NewExecutionPlanner, etc. keep compiling
// during the Stage 2 migration. See REFACTOR_PLAN.md.
package engine

import "github.com/GrayCodeAI/hawk/internal/engine/planning"

type ExecutionPlan = planning.ExecutionPlan
type ExecutionStep = planning.ExecutionStep
type PlannedCall = planning.PlannedCall
type ExecutionPlanner = planning.ExecutionPlanner
type Task = planning.Task
type TaskPlan = planning.TaskPlan
type TaskDecomposer = planning.TaskDecomposer
type Subtask = planning.Subtask
type PlanState = planning.PlanState
type GoalStatus = planning.GoalStatus
type Goal = planning.Goal
type GoalEvent = planning.GoalEvent
type GoalTracker = planning.GoalTracker
type GoalOption = planning.GoalOption
type SuggestedTask = planning.SuggestedTask
type TaskQueue = planning.TaskQueue
type ActionRequired = planning.ActionRequired
type FormField = planning.FormField
type FormResponse = planning.FormResponse
type ActionManager = planning.ActionManager

var NewExecutionPlanner = planning.NewExecutionPlanner
var NewTaskDecomposer = planning.NewTaskDecomposer
var NewPlanState = planning.NewPlanState
var DecomposePrompt = planning.DecomposePrompt
var ParseSubtasks = planning.ParseSubtasks
var WithPriority = planning.WithPriority
var WithBudget = planning.WithBudget
var WithDependencies = planning.WithDependencies
var WithTags = planning.WithTags
var NewGoalTracker = planning.NewGoalTracker
var NewTaskQueue = planning.NewTaskQueue
var ScanGitTasks = planning.ScanGitTasks
var ScanTODOs = planning.ScanTODOs
var ScanTestFailures = planning.ScanTestFailures
var FormatTasks = planning.FormatTasks
var NewActionManager = planning.NewActionManager
var Validate = planning.Validate
var BuildFormPrompt = planning.BuildFormPrompt
var FormatResponse = planning.FormatResponse
