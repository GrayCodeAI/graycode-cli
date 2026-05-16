package tool

import (
	"context"
	"testing"
)

func TestEnterPlanMode(t *testing.T) {
	tool := EnterPlanModeTool{}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("should return confirmation")
	}
	if !IsPlanMode() {
		t.Error("should be in plan mode after enter")
	}
}

func TestExitPlanMode(t *testing.T) {
	enter := EnterPlanModeTool{}
	_, _ = enter.Execute(context.Background(), nil)

	tool := ExitPlanModeTool{}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("should return confirmation")
	}
	if IsPlanMode() {
		t.Error("should not be in plan mode after exit")
	}
}

func TestPlanMode_Metadata(t *testing.T) {
	t.Parallel()
	e := EnterPlanModeTool{}
	if e.Name() != "EnterPlanMode" {
		t.Errorf("Name = %q", e.Name())
	}
	x := ExitPlanModeTool{}
	if x.Name() != "ExitPlanMode" {
		t.Errorf("Name = %q", x.Name())
	}
}
