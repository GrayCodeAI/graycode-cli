package tool

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTeamCreateTool(t *testing.T) {
	// Reset registry
	globalTeamRegistry = &TeamRegistry{teams: make(map[string]*TeamConfig)}

	input, _ := json.Marshal(map[string]string{
		"team_name":   "test-team",
		"description": "A test team",
	})
	result, err := (TeamCreateTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		TeamName string `json:"team_name"`
	}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.TeamName != "test-team" {
		t.Fatalf("expected 'test-team', got %q", resp.TeamName)
	}
}

func TestTeamCreateTool_Duplicate(t *testing.T) {
	globalTeamRegistry = &TeamRegistry{teams: make(map[string]*TeamConfig)}

	input, _ := json.Marshal(map[string]string{"team_name": "dupe"})
	(TeamCreateTool{}).Execute(context.Background(), input)
	_, err := (TeamCreateTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for duplicate team")
	}
}

func TestTeamDeleteTool(t *testing.T) {
	globalTeamRegistry = &TeamRegistry{teams: make(map[string]*TeamConfig)}
	globalTeamRegistry.Create("del-team", "to delete", "")

	input, _ := json.Marshal(map[string]string{"team_name": "del-team"})
	result, err := (TeamDeleteTool{}).Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp struct {
		Success bool `json:"success"`
	}
	json.Unmarshal([]byte(result), &resp)
	if !resp.Success {
		t.Fatal("expected success=true")
	}
}

func TestTeamDeleteTool_NotFound(t *testing.T) {
	globalTeamRegistry = &TeamRegistry{teams: make(map[string]*TeamConfig)}
	input, _ := json.Marshal(map[string]string{"team_name": "ghost"})
	_, err := (TeamDeleteTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for nonexistent team")
	}
}

func TestTeamDeleteTool_ActiveMembers(t *testing.T) {
	globalTeamRegistry = &TeamRegistry{teams: make(map[string]*TeamConfig)}
	globalTeamRegistry.Create("active-team", "has members", "")
	globalTeamRegistry.AddMember("active-team", "worker-1")

	input, _ := json.Marshal(map[string]string{"team_name": "active-team"})
	_, err := (TeamDeleteTool{}).Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when team has active members")
	}
}
