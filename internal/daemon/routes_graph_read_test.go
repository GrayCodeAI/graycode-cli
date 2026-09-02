package daemon

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GrayCodeAI/hawk/internal/executiongraph"
)

func getGraphRead(t *testing.T, addr, path string) *http.Response {
	t.Helper()
	resp, err := http.Get("http://" + addr + path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	return resp
}

func decodeGraphRead(t *testing.T, resp *http.Response) GraphReadResponse {
	t.Helper()
	defer resp.Body.Close()
	var out GraphReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	return out
}

// graphSyncBodyWithSession marshals an export into the envelope with a session.
func graphSyncBodyWithSession(t *testing.T, syncID, sessionID string, export executiongraph.Export) []byte {
	t.Helper()
	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	env := GraphSyncRequest{SyncID: syncID, ProjectID: "proj", SessionID: sessionID, Graph: raw}
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return out
}

func TestDaemon_GraphRead_Empty(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	resp := getGraphRead(t, addr, "/v1/projects/proj/graph")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	out := decodeGraphRead(t, resp)
	if out.Graph.SchemaVersion != "graycode-local.graph/v1" {
		t.Errorf("schema_version = %q, want graycode-local.graph/v1", out.Graph.SchemaVersion)
	}
	if out.Graph.Scope["project_id"] != "proj" {
		t.Errorf("scope.project_id = %q, want proj", out.Graph.Scope["project_id"])
	}
	if len(out.Graph.Nodes) != 0 || len(out.Graph.Edges) != 0 || len(out.Graph.Events) != 0 {
		t.Errorf("expected empty graph, got nodes=%d edges=%d events=%d", len(out.Graph.Nodes), len(out.Graph.Edges), len(out.Graph.Events))
	}
	if out.Limits.PerFactType != defaultGraphReadLimit {
		t.Errorf("limits.perFactType = %d, want %d", out.Limits.PerFactType, defaultGraphReadLimit)
	}
}

func TestDaemon_GraphRead_ReturnsIngestedFacts(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-r1", validTestExport()))
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("sync expected 202, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	out := decodeGraphRead(t, getGraphRead(t, addr, "/v1/projects/proj/graph"))
	if len(out.Graph.Nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(out.Graph.Nodes))
	}
	if len(out.Graph.Edges) != 1 {
		t.Errorf("edges = %d, want 1", len(out.Graph.Edges))
	}
	if len(out.Graph.Events) != 1 {
		t.Errorf("events = %d, want 1", len(out.Graph.Events))
	}
	var firstNode map[string]any
	if err := json.Unmarshal(out.Graph.Nodes[0], &firstNode); err != nil {
		t.Fatalf("unmarshal node: %v", err)
	}
	if firstNode["id"] != "n1" {
		t.Errorf("first node id = %v, want n1", firstNode["id"])
	}
}

func TestDaemon_GraphRead_SessionFilter(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	sync := postGraphSync(t, addr, graphSyncBodyWithSession(t, "sync-s1", "sess-a", validTestExport()))
	if sync.StatusCode != http.StatusAccepted {
		t.Fatalf("session sync expected 202, got %d", sync.StatusCode)
	}
	sync.Body.Close()

	// Matching session returns the facts.
	matching := decodeGraphRead(t, getGraphRead(t, addr, "/v1/projects/proj/graph?sessionId=sess-a"))
	if len(matching.Graph.Nodes) != 2 {
		t.Errorf("session-filtered nodes = %d, want 2", len(matching.Graph.Nodes))
	}
	// Non-matching session returns empty.
	other := decodeGraphRead(t, getGraphRead(t, addr, "/v1/projects/proj/graph?sessionId=sess-b"))
	if len(other.Graph.Nodes) != 0 {
		t.Errorf("non-matching session nodes = %d, want 0", len(other.Graph.Nodes))
	}
}

func TestDaemon_GraphRead_ProjectFilter(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	sync := postGraphSync(t, addr, graphSyncBody(t, "sync-p1", validTestExport()))
	if sync.StatusCode != http.StatusAccepted {
		t.Fatalf("sync expected 202, got %d", sync.StatusCode)
	}
	sync.Body.Close()

	out := decodeGraphRead(t, getGraphRead(t, addr, "/v1/projects/other/graph"))
	if len(out.Graph.Nodes) != 0 {
		t.Errorf("other-project nodes = %d, want 0", len(out.Graph.Nodes))
	}
}

func TestDaemon_GraphRead_InvalidLimit(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	for _, q := range []string{"limit=0", "limit=-1", "limit=abc"} {
		resp := getGraphRead(t, addr, "/v1/projects/proj/graph?"+q)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", q, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

func TestDaemon_GraphRead_LimitClamp(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	resp := getGraphRead(t, addr, "/v1/projects/proj/graph?limit=10000")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	out := decodeGraphRead(t, resp)
	if out.Limits.PerFactType != maxGraphReadLimit {
		t.Errorf("limits.perFactType = %d, want %d", out.Limits.PerFactType, maxGraphReadLimit)
	}
}
