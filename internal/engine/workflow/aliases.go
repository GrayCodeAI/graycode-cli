// Package workflow is the Stage-1 namespace for workflow + workspace +
// trajectory types in package engine. See ../REFACTOR_PLAN.md.
package workflow

import "context"

// Step is one node in a Workflow.
type Step = WorkflowStep

// Result is the outcome of running a Workflow.
type Result = WorkflowResult

// Engine executes Workflows against a model-call function.
type Engine = WorkflowEngine

// NewEngine returns a workflow engine that delegates step execution to the
// provided function (which typically wraps an LLM call).
func NewEngine(executeFn func(ctx context.Context, agent, prompt string) (string, error)) *Engine {
	return NewWorkflowEngine(executeFn)
}
