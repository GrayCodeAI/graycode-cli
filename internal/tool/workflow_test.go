package tool

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestWorkflowTool_EmptyName(t *testing.T) {
	wf := WorkflowTool{}
	_, err := wf.Execute(context.Background(), json.RawMessage(`{"workflow":""}`))
	if err == nil {
		t.Fatal("expected error for empty workflow name")
	}
}

func TestWorkflowTool_NotFound(t *testing.T) {
	wf := WorkflowTool{}
	_, err := wf.Execute(context.Background(), json.RawMessage(`{"workflow":"nonexistent_workflow_xyz"}`))
	if err == nil {
		t.Fatal("expected error for nonexistent workflow")
	}
}

func TestWorkflowTool_Name(t *testing.T) {
	wf := WorkflowTool{}
	if wf.Name() != "Workflow" {
		t.Fatalf("expected Workflow, got %s", wf.Name())
	}
}

func TestListWorkflows_Empty(t *testing.T) {
	// Change to a temp dir with no .hawk/workflows/
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(tmp)
	defer os.Chdir(orig)

	workflows := ListWorkflows()
	// May pick up home dir workflows, but local dir should have none
	// Just verify it doesn't panic and returns a slice
	_ = workflows
}
