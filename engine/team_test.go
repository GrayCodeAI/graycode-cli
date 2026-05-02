package engine

import (
	"strings"
	"testing"
)

func TestAgentTeam_AddMember(t *testing.T) {
	team := NewAgentTeam()
	team.AddMember("alice", "researcher", nil)
	team.AddMember("bob", "implementer", nil)

	m := team.GetMember("alice")
	if m == nil {
		t.Fatal("expected alice to be a member")
	}
	if m.Role != "researcher" {
		t.Errorf("expected role researcher, got %s", m.Role)
	}
	if m.Status != "idle" {
		t.Errorf("expected status idle, got %s", m.Status)
	}

	if team.GetMember("charlie") != nil {
		t.Error("expected nil for non-existent member")
	}
}

func TestAgentTeam_CreateAndAssignTask(t *testing.T) {
	team := NewAgentTeam()
	team.AddMember("alice", "researcher", nil)

	taskID := team.CreateTask("research API design", nil)
	if taskID == "" {
		t.Fatal("expected non-empty task ID")
	}

	team.AssignTask(taskID, "alice")

	m := team.GetMember("alice")
	if m.Status != "working" {
		t.Errorf("expected alice working, got %s", m.Status)
	}
	if m.Assigned != taskID {
		t.Errorf("expected alice assigned to %s, got %s", taskID, m.Assigned)
	}
}

func TestAgentTeam_CompleteTask(t *testing.T) {
	team := NewAgentTeam()
	team.AddMember("alice", "researcher", nil)

	taskID := team.CreateTask("write tests", nil)
	team.AssignTask(taskID, "alice")
	team.CompleteTask(taskID, "all tests pass")

	m := team.GetMember("alice")
	if m.Status != "idle" {
		t.Errorf("expected alice idle after completing task, got %s", m.Status)
	}
	if m.Assigned != "" {
		t.Errorf("expected alice unassigned, got %s", m.Assigned)
	}
}

func TestAgentTeam_Messaging(t *testing.T) {
	team := NewAgentTeam()
	team.AddMember("alice", "researcher", nil)
	team.AddMember("bob", "implementer", nil)

	// Direct message
	team.SendMessage(TeamMessage{From: "alice", To: "bob", Content: "found the bug"})

	// Broadcast
	team.SendMessage(TeamMessage{From: "alice", To: "*", Content: "status update"})

	// Bob should receive both
	bobMsgs := team.PendingMessages("bob")
	if len(bobMsgs) != 2 {
		t.Fatalf("expected 2 messages for bob, got %d", len(bobMsgs))
	}
	if bobMsgs[0].Content != "found the bug" {
		t.Errorf("expected 'found the bug', got %q", bobMsgs[0].Content)
	}
	if bobMsgs[1].Content != "status update" {
		t.Errorf("expected 'status update', got %q", bobMsgs[1].Content)
	}

	// Alice should have no direct messages left (broadcast already consumed by bob)
	aliceMsgs := team.PendingMessages("alice")
	if len(aliceMsgs) != 0 {
		t.Errorf("expected 0 messages for alice, got %d", len(aliceMsgs))
	}
}

func TestAgentTeam_Status(t *testing.T) {
	team := NewAgentTeam()
	team.AddMember("alice", "researcher", nil)

	taskID := team.CreateTask("analyze code", []string{})
	team.AssignTask(taskID, "alice")

	status := team.Status()
	if !strings.Contains(status, "alice") {
		t.Error("status should contain alice")
	}
	if !strings.Contains(status, "researcher") {
		t.Error("status should contain role")
	}
	if !strings.Contains(status, "analyze code") {
		t.Error("status should contain task description")
	}
	if !strings.Contains(status, "in_progress") {
		t.Error("status should show in_progress task")
	}
}

func TestAgentTeam_TaskDependencies(t *testing.T) {
	team := NewAgentTeam()
	team.AddMember("alice", "researcher", nil)
	team.AddMember("bob", "implementer", nil)

	task1 := team.CreateTask("design API", nil)
	task2 := team.CreateTask("implement API", []string{task1})

	status := team.Status()
	if !strings.Contains(status, task1) {
		t.Error("status should contain task1 ID")
	}
	if !strings.Contains(status, "deps=") {
		t.Error("status should show dependencies for task2")
	}

	// Assign and complete first task
	team.AssignTask(task1, "alice")
	team.CompleteTask(task1, "design done")

	// Assign second task
	team.AssignTask(task2, "bob")
	m := team.GetMember("bob")
	if m.Status != "working" {
		t.Errorf("expected bob working, got %s", m.Status)
	}
}
