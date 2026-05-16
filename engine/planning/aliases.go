// Package planning is the Stage-1 namespace for task planning, decomposition, goals, and suggested tasks.
// See ../REFACTOR_PLAN.md.
package planning

import "github.com/GrayCodeAI/hawk/engine"

type ExecutionPlan = engine.ExecutionPlan
type ExecutionStep = engine.ExecutionStep
type PlannedCall = engine.PlannedCall
type ExecutionPlanner = engine.ExecutionPlanner
type Task = engine.Task
type TaskPlan = engine.TaskPlan
type TaskDecomposer = engine.TaskDecomposer
type Subtask = engine.Subtask
type PlanState = engine.PlanState
type GoalStatus = engine.GoalStatus
type Goal = engine.Goal
type GoalEvent = engine.GoalEvent
type GoalTracker = engine.GoalTracker
type GoalOption = engine.GoalOption
type SuggestedTask = engine.SuggestedTask
type TaskQueue = engine.TaskQueue
type ActionRequired = engine.ActionRequired
type FormField = engine.FormField
type FormResponse = engine.FormResponse
type ActionManager = engine.ActionManager

var NewExecutionPlanner = engine.NewExecutionPlanner
var NewTaskDecomposer = engine.NewTaskDecomposer
var NewPlanState = engine.NewPlanState
var DecomposePrompt = engine.DecomposePrompt
var ParseSubtasks = engine.ParseSubtasks
var WithPriority = engine.WithPriority
var WithBudget = engine.WithBudget
var WithDependencies = engine.WithDependencies
var WithTags = engine.WithTags
var NewGoalTracker = engine.NewGoalTracker
var NewTaskQueue = engine.NewTaskQueue
var ScanGitTasks = engine.ScanGitTasks
var ScanTODOs = engine.ScanTODOs
var ScanTestFailures = engine.ScanTestFailures
var FormatTasks = engine.FormatTasks
var NewActionManager = engine.NewActionManager
var Validate = engine.Validate
var BuildFormPrompt = engine.BuildFormPrompt
var FormatResponse = engine.FormatResponse
