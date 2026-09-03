package cmd

import (
	"testing"

	"github.com/GrayCodeAI/graycode-cli/internal/ui/icons"
)

func TestAgentStateString(t *testing.T) {
	tests := []struct {
		state AgentState
		want  string
	}{
		{AgentIdle, icons.Hourglass() + " Idle"},
		{AgentRunning, icons.Refresh() + " Running"},
		{AgentDone, icons.CheckDecagram() + " Done"},
		{AgentFailed, icons.Cancel() + " Failed"},
	}
	for _, tc := range tests {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("AgentState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestNewAgentPane(t *testing.T) {
	pane := NewAgentPane("1", "Fix auth bug", 80, 24)
	if pane.ID != "1" {
		t.Errorf("ID = %q, want %q", pane.ID, "1")
	}
	if pane.Task != "Fix auth bug" {
		t.Errorf("Task = %q, want %q", pane.Task, "Fix auth bug")
	}
	if pane.State != AgentIdle {
		t.Errorf("State = %v, want AgentIdle", pane.State)
	}
}

func TestAgentPaneAppend(t *testing.T) {
	pane := NewAgentPane("1", "Test", 80, 24)
	pane.Append("Line 1")
	pane.Append("Line 2")

	if len(pane.Output) != 2 {
		t.Errorf("Output length = %d, want 2", len(pane.Output))
	}
}

func TestAgentPaneSetState(t *testing.T) {
	pane := NewAgentPane("1", "Test", 80, 24)
	pane.SetState(AgentRunning)

	if pane.State != AgentRunning {
		t.Errorf("State = %v, want AgentRunning", pane.State)
	}
	if pane.startTime.IsZero() {
		t.Error("startTime should be set when running")
	}

	pane.SetState(AgentDone)
	if pane.endTime.IsZero() {
		t.Error("endTime should be set when done")
	}
}

func TestNewAgentGrid(t *testing.T) {
	tasks := []string{"Task 1", "Task 2", "Task 3"}
	grid := NewAgentGrid(tasks, 160, 48)

	if len(grid.panes) != 3 {
		t.Errorf("panes = %d, want 3", len(grid.panes))
	}
}

func TestAgentGridGetPane(t *testing.T) {
	tasks := []string{"Task 1", "Task 2"}
	grid := NewAgentGrid(tasks, 160, 48)

	pane := grid.GetPane("1")
	if pane == nil {
		t.Fatal("GetPane(1) returned nil")
	}
	if pane.Task != "Task 1" {
		t.Errorf("Task = %q, want %q", pane.Task, "Task 1")
	}

	if grid.GetPane("99") != nil {
		t.Error("GetPane(99) should return nil")
	}
}

func TestAgentGridAllDone(t *testing.T) {
	tasks := []string{"Task 1", "Task 2"}
	grid := NewAgentGrid(tasks, 160, 48)

	if grid.AllDone() {
		t.Error("AllDone should be false when idle")
	}

	grid.panes[0].SetState(AgentDone)
	if grid.AllDone() {
		t.Error("AllDone should be false when one is idle")
	}

	grid.panes[1].SetState(AgentDone)
	if !grid.AllDone() {
		t.Error("AllDone should be true when all done")
	}
}

func TestAgentGridSummary(t *testing.T) {
	tasks := []string{"Task 1", "Task 2", "Task 3"}
	grid := NewAgentGrid(tasks, 160, 48)

	grid.panes[0].SetState(AgentDone)
	grid.panes[1].SetState(AgentFailed)
	grid.panes[2].SetState(AgentRunning)

	summary := grid.Summary()
	if summary != "1 done, 1 failed, 1 running" {
		t.Errorf("Summary = %q, want %q", summary, "1 done, 1 failed, 1 running")
	}
}

func TestAgentGridRender(t *testing.T) {
	tasks := []string{"Task 1", "Task 2"}
	grid := NewAgentGrid(tasks, 160, 48)

	rendered := grid.Render()
	if rendered == "" {
		t.Error("Render should not return empty string")
	}
}
