package engine

import (
	"strings"
	"testing"
)

func TestParseSubtasks_Basic(t *testing.T) {
	input := `1. Create the models
   Define the data structures for the project
   Files: model.go, types.go

2. Implement the API
   Build REST endpoints for CRUD operations
   Files: api.go, handler.go

3. Write tests
   Add unit tests for all handlers
   Files: api_test.go`

	subtasks := ParseSubtasks(input)
	if len(subtasks) != 3 {
		t.Fatalf("expected 3 subtasks, got %d", len(subtasks))
	}

	if subtasks[0].ID != 1 || subtasks[0].Title != "Create the models" {
		t.Errorf("subtask 1: got ID=%d Title=%q", subtasks[0].ID, subtasks[0].Title)
	}
	if subtasks[0].Description != "Define the data structures for the project" {
		t.Errorf("subtask 1 description: %q", subtasks[0].Description)
	}
	if len(subtasks[0].Files) != 2 || subtasks[0].Files[0] != "model.go" || subtasks[0].Files[1] != "types.go" {
		t.Errorf("subtask 1 files: %v", subtasks[0].Files)
	}

	if subtasks[1].ID != 2 || subtasks[1].Title != "Implement the API" {
		t.Errorf("subtask 2: got ID=%d Title=%q", subtasks[1].ID, subtasks[1].Title)
	}

	if subtasks[2].ID != 3 || len(subtasks[2].Files) != 1 {
		t.Errorf("subtask 3: got ID=%d Files=%v", subtasks[2].ID, subtasks[2].Files)
	}

	for _, st := range subtasks {
		if st.Status != "pending" {
			t.Errorf("subtask %d should be pending, got %s", st.ID, st.Status)
		}
	}
}

func TestParseSubtasks_Empty(t *testing.T) {
	subtasks := ParseSubtasks("")
	if len(subtasks) != 0 {
		t.Errorf("expected 0 subtasks from empty input, got %d", len(subtasks))
	}
}

func TestParseSubtasks_NoFiles(t *testing.T) {
	input := `1. Do something
   A description without files`

	subtasks := ParseSubtasks(input)
	if len(subtasks) != 1 {
		t.Fatalf("expected 1 subtask, got %d", len(subtasks))
	}
	if len(subtasks[0].Files) != 0 {
		t.Errorf("expected no files, got %v", subtasks[0].Files)
	}
}

func TestParseSubtasks_ConsecutiveNoBlankLine(t *testing.T) {
	input := `1. First task
   Description one
   Files: a.go
2. Second task
   Description two
   Files: b.go`

	subtasks := ParseSubtasks(input)
	if len(subtasks) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(subtasks))
	}
	if subtasks[0].Title != "First task" {
		t.Errorf("first title: %q", subtasks[0].Title)
	}
	if subtasks[1].Title != "Second task" {
		t.Errorf("second title: %q", subtasks[1].Title)
	}
}

func TestNewPlanState(t *testing.T) {
	ps := NewPlanState("test plan")
	if ps.Name != "test plan" {
		t.Errorf("expected name 'test plan', got %q", ps.Name)
	}
	if !ps.Active {
		t.Error("new plan should be active")
	}
	if len(ps.Subtasks) != 0 {
		t.Error("new plan should have no subtasks")
	}
}

func TestPlanState_Next(t *testing.T) {
	ps := NewPlanState("test")
	ps.Subtasks = []Subtask{
		{ID: 1, Title: "first", Status: "pending"},
		{ID: 2, Title: "second", Status: "pending"},
	}

	next := ps.Next()
	if next == nil {
		t.Fatal("expected non-nil subtask")
	}
	if next.ID != 1 {
		t.Errorf("expected ID 1, got %d", next.ID)
	}
	if next.Status != "in_progress" {
		t.Errorf("expected in_progress, got %s", next.Status)
	}
	if ps.Current != 1 {
		t.Errorf("current should be 1, got %d", ps.Current)
	}
}

func TestPlanState_Next_AllDone(t *testing.T) {
	ps := NewPlanState("test")
	ps.Subtasks = []Subtask{
		{ID: 1, Title: "first", Status: "done"},
		{ID: 2, Title: "second", Status: "skipped"},
	}

	if ps.Next() != nil {
		t.Error("expected nil when all subtasks are done/skipped")
	}
}

func TestPlanState_MarkDone(t *testing.T) {
	ps := NewPlanState("test")
	ps.Subtasks = []Subtask{
		{ID: 1, Title: "first", Status: "in_progress"},
		{ID: 2, Title: "second", Status: "pending"},
	}

	ps.MarkDone(1)
	if ps.Subtasks[0].Status != "done" {
		t.Errorf("expected done, got %s", ps.Subtasks[0].Status)
	}
}

func TestPlanState_Skip(t *testing.T) {
	ps := NewPlanState("test")
	ps.Subtasks = []Subtask{
		{ID: 1, Title: "first", Status: "pending"},
	}

	ps.Skip(1)
	if ps.Subtasks[0].Status != "skipped" {
		t.Errorf("expected skipped, got %s", ps.Subtasks[0].Status)
	}
}

func TestPlanState_Progress(t *testing.T) {
	ps := NewPlanState("test")
	ps.Subtasks = []Subtask{
		{ID: 1, Title: "first", Status: "done"},
		{ID: 2, Title: "second", Status: "done"},
		{ID: 3, Title: "third", Status: "pending"},
	}

	progress := ps.Progress()
	if progress != "2/3 subtasks complete" {
		t.Errorf("expected '2/3 subtasks complete', got %q", progress)
	}
}

func TestPlanState_Format(t *testing.T) {
	ps := NewPlanState("build feature")
	ps.Subtasks = []Subtask{
		{ID: 1, Title: "Design", Description: "Plan the architecture", Status: "done", Files: []string{"design.md"}},
		{ID: 2, Title: "Implement", Description: "Write the code", Status: "in_progress", Files: []string{"main.go"}},
		{ID: 3, Title: "Test", Status: "pending"},
	}

	formatted := ps.Format()
	if !strings.Contains(formatted, "Plan: build feature") {
		t.Error("should contain plan name")
	}
	if !strings.Contains(formatted, "1/3 subtasks complete") {
		t.Error("should contain progress")
	}
	if !strings.Contains(formatted, "[x] 1. Design") {
		t.Error("should show done icon for subtask 1")
	}
	if !strings.Contains(formatted, "[>] 2. Implement") {
		t.Error("should show in_progress icon for subtask 2")
	}
	if !strings.Contains(formatted, "[ ] 3. Test") {
		t.Error("should show pending icon for subtask 3")
	}
}

func TestPlanState_Format_Empty(t *testing.T) {
	ps := NewPlanState("empty plan")
	formatted := ps.Format()
	if !strings.Contains(formatted, "no subtasks") {
		t.Errorf("expected 'no subtasks', got %q", formatted)
	}
}

func TestDecomposePrompt(t *testing.T) {
	prompt := DecomposePrompt()
	if prompt == "" {
		t.Error("decompose prompt should not be empty")
	}
	if !strings.Contains(prompt, "subtask") {
		t.Error("prompt should mention subtasks")
	}
	if !strings.Contains(prompt, "Files:") {
		t.Error("prompt should mention Files: format")
	}
}
