package cmd

import (
	"strings"
	"testing"
)

func TestRenderAgentStatusPanel_Idle(t *testing.T) {
	out := renderAgentStatusPanel(HUDData{MissionStatus: "idle"}, 80)
	if !strings.Contains(out, "Agent Status HUD") {
		t.Error("expected HUD header")
	}
	if !strings.Contains(out, "no active mission") {
		t.Error("expected idle mission indicator")
	}
	if !strings.Contains(out, "yaad not connected") {
		t.Error("expected memory-not-connected indicator")
	}
}

func TestRenderAgentStatusPanel_WithData(t *testing.T) {
	data := HUDData{
		MissionID:      "abc123",
		MissionStatus:  "executing",
		FeaturesTotal:  3,
		FeaturesDone:   1,
		FeaturesFailed: 0,
		ActiveAgents: []HUDAgent{
			{ID: "w1", Task: "implement feature A", Status: "running"},
		},
		RecentMessages: []HUDMessage{
			{From: "w1", Topic: "progress", Content: "50% done"},
		},
		MemoryReady:    true,
		MemoryNodes:    42,
		MemoryEdges:    100,
		MemorySessions: 3,
	}
	out := renderAgentStatusPanel(data, 80)
	for _, want := range []string{"abc123", "executing", "w1", "progress", "42"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected HUD output to contain %q", want)
		}
	}
}

func TestTruncateHUD(t *testing.T) {
	if got := truncateHUD("hello world", 8); got != "hello..." {
		t.Errorf("expected 'hello...', got %q", got)
	}
	if got := truncateHUD("short", 20); got != "short" {
		t.Errorf("expected 'short', got %q", got)
	}
	if got := truncateHUD("line1\nline2", 50); strings.Contains(got, "\n") {
		t.Errorf("expected newlines stripped, got %q", got)
	}
}
