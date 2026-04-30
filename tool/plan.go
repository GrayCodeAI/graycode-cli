package tool

import (
	"context"
	"encoding/json"
	"sync/atomic"
)

var planMode atomic.Bool

// IsPlanMode returns whether plan mode is active.
func IsPlanMode() bool { return planMode.Load() }

type EnterPlanModeTool struct{}

func (EnterPlanModeTool) Name() string { return "enter_plan_mode" }
func (EnterPlanModeTool) Description() string {
	return "Enter plan mode. In plan mode, you should only read files and discuss plans — do not write files or run commands that modify state. Use this when the user asks you to plan before implementing."
}
func (EnterPlanModeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}
func (EnterPlanModeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	planMode.Store(true)
	return "Entered plan mode. I will only read and discuss — no modifications until you say to proceed.", nil
}

type ExitPlanModeTool struct{}

func (ExitPlanModeTool) Name() string { return "exit_plan_mode" }
func (ExitPlanModeTool) Description() string {
	return "Exit plan mode and begin implementation. Use this when the user approves the plan and wants you to start making changes."
}
func (ExitPlanModeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
}
func (ExitPlanModeTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	planMode.Store(false)
	return "Exited plan mode. Ready to implement.", nil
}
