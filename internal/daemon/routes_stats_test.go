package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	analytics "github.com/GrayCodeAI/graycode-cli/internal/observability"
	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

func TestDaemon_Stats_Aggregation(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())

	now := time.Now()
	traces := []*analytics.SessionTrace{
		{SessionID: "s1", StartTime: now, Model: "claude-opus", MessageCount: 4, ToolCalls: 2, CostUSD: 0.50},
		{SessionID: "s2", StartTime: now.Add(-time.Hour), Model: "claude-opus", MessageCount: 6, ToolCalls: 1, CostUSD: 0.25},
		{SessionID: "s3", StartTime: now.AddDate(0, 0, -1), Model: "claude-sonnet", MessageCount: 2, ToolCalls: 0, CostUSD: 0.10},
		// Outside the default 30-day window — must not be counted.
		{SessionID: "s4", StartTime: now.AddDate(0, 0, -40), Model: "claude-sonnet", MessageCount: 100, ToolCalls: 50, CostUSD: 9.99},
	}
	for _, tr := range traces {
		if err := analytics.SaveTrace(tr); err != nil {
			t.Fatalf("SaveTrace: %v", err)
		}
	}

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/stats")
	if err != nil {
		t.Fatalf("GET /v1/stats failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var stats StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if stats.TotalSessions != 3 {
		t.Errorf("TotalSessions = %d, want 3 (s4 is outside the 30-day window)", stats.TotalSessions)
	}
	if stats.TotalMessages != 12 {
		t.Errorf("TotalMessages = %d, want 12", stats.TotalMessages)
	}
	if stats.TotalToolCalls != 3 {
		t.Errorf("TotalToolCalls = %d, want 3", stats.TotalToolCalls)
	}
	wantCost := 0.85
	if diff := stats.TotalCostUSD - wantCost; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("TotalCostUSD = %v, want %v", stats.TotalCostUSD, wantCost)
	}
	if stats.ActiveDays != 2 {
		t.Errorf("ActiveDays = %d, want 2 (today + yesterday)", stats.ActiveDays)
	}
	if len(stats.Models) != 2 {
		t.Fatalf("len(Models) = %d, want 2", len(stats.Models))
	}
	byModel := map[string]ModelStatResp{}
	for _, m := range stats.Models {
		byModel[m.Model] = m
	}
	if byModel["claude-opus"].Requests != 2 {
		t.Errorf("claude-opus Requests = %d, want 2", byModel["claude-opus"].Requests)
	}
	if byModel["claude-sonnet"].Requests != 1 {
		t.Errorf("claude-sonnet Requests = %d, want 1", byModel["claude-sonnet"].Requests)
	}
}

func TestDaemon_Stats_DaysParam(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())

	old := &analytics.SessionTrace{
		SessionID: "old", StartTime: time.Now().AddDate(0, 0, -10), Model: "m", MessageCount: 1,
	}
	if err := analytics.SaveTrace(old); err != nil {
		t.Fatalf("SaveTrace: %v", err)
	}

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	// Default 30-day window includes it.
	resp, err := http.Get("http://" + addr + "/v1/stats")
	if err != nil {
		t.Fatalf("GET /v1/stats failed: %v", err)
	}
	var stats StatsResponse
	json.NewDecoder(resp.Body).Decode(&stats)
	resp.Body.Close()
	if stats.TotalSessions != 1 {
		t.Fatalf("default window: TotalSessions = %d, want 1", stats.TotalSessions)
	}

	// A 5-day window excludes a trace from 10 days ago.
	resp2, err := http.Get("http://" + addr + "/v1/stats?days=5")
	if err != nil {
		t.Fatalf("GET /v1/stats?days=5 failed: %v", err)
	}
	defer resp2.Body.Close()
	var stats2 StatsResponse
	if err := json.NewDecoder(resp2.Body).Decode(&stats2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats2.TotalSessions != 0 {
		t.Errorf("days=5 window: TotalSessions = %d, want 0", stats2.TotalSessions)
	}
}

func TestDaemon_Stats_Empty(t *testing.T) {
	t.Setenv("GRAYCODE_STATE_DIR", t.TempDir())

	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	defer srv.Stop(context.Background())

	resp, err := http.Get("http://" + addr + "/v1/stats")
	if err != nil {
		t.Fatalf("GET /v1/stats failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var stats StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.TotalSessions != 0 {
		t.Errorf("TotalSessions = %d, want 0", stats.TotalSessions)
	}
	if len(stats.Models) != 0 {
		t.Errorf("len(Models) = %d, want 0", len(stats.Models))
	}
}
