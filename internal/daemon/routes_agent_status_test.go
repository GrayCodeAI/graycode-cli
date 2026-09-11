package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentStatusStates(t *testing.T) {
	s := New(DefaultConfig(), nil)
	now := time.Now()

	// working: in-flight cancel registered.
	s.sessions.Store("working-1", &Session{ID: "working-1", Agent: "hawk", Turns: 3, LastUsed: now})
	s.cancelMu.Lock()
	s.cancels["working-1"] = &cancelEntry{cancel: func() {}}
	s.cancelMu.Unlock()
	// idle: recent activity, no in-flight generation.
	s.sessions.Store("idle-1", &Session{ID: "idle-1", Turns: 1, LastUsed: now.Add(-2 * time.Minute)})
	// stale: no activity for over 30 minutes.
	s.sessions.Store("stale-1", &Session{ID: "stale-1", LastUsed: now.Add(-45 * time.Minute)})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/agent/status", nil)
	s.handleAgentStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		GeneratedAt string `json:"generated_at"`
		Agents      []struct {
			SessionID string `json:"session_id"`
			State     string `json:"state"`
			Turns     int    `json:"turns"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, a := range body.Agents {
		states[a.SessionID] = a.State
	}
	if states["working-1"] != "working" {
		t.Fatalf("working-1 = %q", states["working-1"])
	}
	if states["idle-1"] != "idle" {
		t.Fatalf("idle-1 = %q", states["idle-1"])
	}
	if states["stale-1"] != "stale" {
		t.Fatalf("stale-1 = %q", states["stale-1"])
	}
}

func TestAgentStatusEmpty(t *testing.T) {
	s := New(DefaultConfig(), nil)
	rec := httptest.NewRecorder()
	s.handleAgentStatus(rec, httptest.NewRequest(http.MethodGet, "/v1/agent/status", nil))
	var body struct {
		Agents []struct{} `json:"agents"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Agents) != 0 {
		t.Fatalf("expected empty agents, got %d", len(body.Agents))
	}
}
