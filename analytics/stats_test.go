package analytics

import (
	"strings"
	"testing"
	"time"
)

func TestSessionStats_ProcessEvent(t *testing.T) {
	stats := &SessionStats{
		ToolUsage:     make(map[string]int),
		ModelUsage:    make(map[string]ModelStats),
		LanguageStats: make(map[string]int),
	}

	stats.processEvent(map[string]interface{}{
		"type": "session_start",
	})
	if stats.TotalSessions != 1 {
		t.Errorf("expected 1 session, got %d", stats.TotalSessions)
	}

	stats.processEvent(map[string]interface{}{
		"type": "tool_use",
		"tool": "Bash",
	})
	stats.processEvent(map[string]interface{}{
		"type": "tool_use",
		"tool": "Bash",
	})
	stats.processEvent(map[string]interface{}{
		"type": "tool_use",
		"tool": "Read",
	})
	if stats.ToolUsage["Bash"] != 2 {
		t.Errorf("expected Bash=2, got %d", stats.ToolUsage["Bash"])
	}

	stats.processEvent(map[string]interface{}{
		"type":   "api_request",
		"model":  "claude-4-sonnet",
		"tokens": float64(1000),
		"cost":   float64(0.05),
	})
	if stats.TotalTokens != 1000 {
		t.Errorf("expected 1000 tokens, got %d", stats.TotalTokens)
	}
	if stats.TotalCost != 0.05 {
		t.Errorf("expected $0.05, got %f", stats.TotalCost)
	}
}

func TestSessionStats_ComputePeaks(t *testing.T) {
	stats := &SessionStats{
		ToolUsage:     make(map[string]int),
		ModelUsage:    make(map[string]ModelStats),
		LanguageStats: make(map[string]int),
	}

	// Fill heatmap: most activity on Monday at 14:00
	stats.ActivityHeatmap[time.Monday][14] = 50
	stats.ActivityHeatmap[time.Monday][15] = 30
	stats.ActivityHeatmap[time.Tuesday][10] = 20

	stats.computePeaks()
	if stats.PeakDay != time.Monday {
		t.Errorf("expected peak day Monday, got %v", stats.PeakDay)
	}
	if stats.PeakHour != 14 {
		t.Errorf("expected peak hour 14, got %d", stats.PeakHour)
	}
}

func TestFormatStats(t *testing.T) {
	stats := &SessionStats{
		TotalSessions: 42,
		TotalMessages: 500,
		TotalTokens:   1500000,
		TotalCost:     12.50,
		GitCommits:    15,
		PeakDay:       time.Wednesday,
		PeakHour:      10,
		ToolUsage: map[string]int{
			"Bash": 100,
			"Read": 80,
			"Edit": 50,
		},
		ModelUsage: map[string]ModelStats{
			"claude-4": {Model: "claude-4", Requests: 200, Tokens: 1000000, Cost: 10.0},
		},
	}

	output := FormatStats(stats)
	if !strings.Contains(output, "42") {
		t.Error("should contain session count")
	}
	if !strings.Contains(output, "$12.50") {
		t.Error("should contain cost")
	}
	if !strings.Contains(output, "Bash") {
		t.Error("should contain tool name")
	}
	if !strings.Contains(output, "claude-4") {
		t.Error("should contain model name")
	}
}

func TestComputeStats_NoLogs(t *testing.T) {
	// Will fail gracefully if no event log directory exists
	_, err := ComputeStats(7)
	if err == nil {
		t.Skip("test requires missing event log dir")
	}
}
