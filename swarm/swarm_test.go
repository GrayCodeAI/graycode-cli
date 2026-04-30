package swarm

import (
	"context"
	"testing"
	"time"
)

func TestCoordinator_AddRemoveTeammate(t *testing.T) {
	c := NewCoordinator("test-team", "leader")

	tm := c.AddTeammate("worker-1", "general-purpose", "#ff0000")
	if tm.Name != "worker-1" {
		t.Fatalf("expected name 'worker-1', got %q", tm.Name)
	}
	if tm.State != AgentStateIdle {
		t.Fatalf("expected state 'idle', got %q", tm.State)
	}
	if tm.TeamName != "test-team" {
		t.Fatalf("expected team 'test-team', got %q", tm.TeamName)
	}

	teammates := c.ListTeammates()
	if len(teammates) != 1 {
		t.Fatalf("expected 1 teammate, got %d", len(teammates))
	}

	c.RemoveTeammate("worker-1")
	teammates = c.ListTeammates()
	if len(teammates) != 0 {
		t.Fatalf("expected 0 teammates after removal, got %d", len(teammates))
	}
}

func TestCoordinator_SetState(t *testing.T) {
	c := NewCoordinator("test-team", "leader")
	c.AddTeammate("worker-1", "general-purpose", "")

	c.SetState("worker-1", AgentStateWorking)
	tm, ok := c.GetTeammate("worker-1")
	if !ok {
		t.Fatal("expected to find teammate")
	}
	if tm.State != AgentStateWorking {
		t.Fatalf("expected state 'working', got %q", tm.State)
	}
}

func TestCoordinator_AssignTask(t *testing.T) {
	c := NewCoordinator("test-team", "leader")
	c.AddTeammate("worker-1", "general-purpose", "")

	c.AssignTask("worker-1", "task_1")
	tm, _ := c.GetTeammate("worker-1")
	if tm.TaskID != "task_1" {
		t.Fatalf("expected taskID 'task_1', got %q", tm.TaskID)
	}
	if tm.State != AgentStateWorking {
		t.Fatalf("expected state 'working', got %q", tm.State)
	}
}

func TestCoordinator_Messaging(t *testing.T) {
	c := NewCoordinator("test-team", "leader")

	c.SendMessage(InternalMessage{
		From:    "leader",
		To:      "worker-1",
		Type:    "task_assignment",
		Content: "please do task_1",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs := c.ReceiveMessages(ctx, "worker-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "please do task_1" {
		t.Fatalf("unexpected content: %q", msgs[0].Content)
	}
}

func TestCoordinator_BroadcastMessage(t *testing.T) {
	c := NewCoordinator("test-team", "leader")

	c.SendMessage(InternalMessage{
		From:    "leader",
		To:      "*",
		Type:    "announcement",
		Content: "all done",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msgs := c.ReceiveMessages(ctx, "any-agent")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 broadcast message, got %d", len(msgs))
	}
}

func TestGetOrCreateCoordinator(t *testing.T) {
	// Clean up global state
	coordinators.Lock()
	coordinators.m = make(map[string]*Coordinator)
	coordinators.Unlock()

	c1 := GetOrCreateCoordinator("my-team", "boss")
	c2 := GetOrCreateCoordinator("my-team", "boss")
	if c1 != c2 {
		t.Fatal("expected same coordinator instance")
	}

	RemoveCoordinator("my-team")
	c3 := GetCoordinator("my-team")
	if c3 != nil {
		t.Fatal("expected nil after removal")
	}
}
