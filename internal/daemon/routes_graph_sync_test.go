package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/hawk/internal/executiongraph"
	"github.com/GrayCodeAI/hawk/internal/testutil"
)

func newGraphSyncTestServer(t *testing.T) string {
	t.Helper()
	srv := New(Config{Port: 0, Host: testutil.LoopbackHost}, nil)
	addr := startTestDaemon(t, srv)
	t.Cleanup(func() { srv.Stop(context.Background()) })
	return addr
}

// validTestExport returns a self-contained portable graph: two nodes, one edge
// referencing both, and one event referencing the first node.
func validTestExport() executiongraph.Export {
	now := time.Now().UTC()
	return executiongraph.Export{
		SchemaVersion: executiongraph.SchemaVersion,
		GeneratedAt:   now,
		Scope:         graphcontracts.Scope{ProjectID: "proj"},
		Nodes: []graphcontracts.Node{
			{ID: "n1", Kind: graphcontracts.NodeSystem, CreatedAt: now, Provenance: graphcontracts.Provenance{Producer: "test"}},
			{ID: "n2", Kind: graphcontracts.NodeKnowledge, CreatedAt: now, Provenance: graphcontracts.Provenance{Producer: "test"}},
		},
		Edges: []graphcontracts.Edge{
			{ID: "e1", Kind: graphcontracts.EdgeReferences, From: graphcontracts.Ref{Kind: graphcontracts.NodeSystem, ID: "n1"}, To: graphcontracts.Ref{Kind: graphcontracts.NodeKnowledge, ID: "n2"}, CreatedAt: now, Provenance: graphcontracts.Provenance{Producer: "test"}},
		},
		Events: []graphcontracts.Event{
			{ID: "ev1", Type: graphcontracts.EventCreated, Subject: graphcontracts.Ref{Kind: graphcontracts.NodeSystem, ID: "n1"}, OccurredAt: now, Provenance: graphcontracts.Provenance{Producer: "test"}},
		},
	}
}

// graphSyncBody marshals an export into the /v1/graph/sync envelope, returning
// the raw request body.
func graphSyncBody(t *testing.T, syncID string, export executiongraph.Export) []byte {
	t.Helper()
	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	env := GraphSyncRequest{SyncID: syncID, ProjectID: "proj", Graph: raw}
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return out
}

func postGraphSync(t *testing.T, addr string, body []byte) *http.Response {
	t.Helper()
	resp, err := http.Post("http://"+addr+"/v1/graph/sync", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/graph/sync failed: %v", err)
	}
	return resp
}

func decodeGraphSyncResponse(t *testing.T, resp *http.Response) GraphSyncResponse {
	t.Helper()
	defer resp.Body.Close()
	var out GraphSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestDaemon_GraphSync_Accepted(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-1", validTestExport()))

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	out := decodeGraphSyncResponse(t, resp)
	if !out.Accepted || out.Duplicate {
		t.Fatalf("expected accepted=true duplicate=false, got %+v", out)
	}
	if out.Facts != 4 {
		t.Fatalf("expected 4 facts, got %d", out.Facts)
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(out.GraphDigest) {
		t.Fatalf("expected 64-hex digest, got %q", out.GraphDigest)
	}
}

func TestDaemon_GraphSync_Duplicate(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	body := graphSyncBody(t, "sync-dup", validTestExport())

	first := postGraphSync(t, addr, body)
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first sync expected 202, got %d", first.StatusCode)
	}
	firstOut := decodeGraphSyncResponse(t, first)

	second := postGraphSync(t, addr, body)
	if second.StatusCode != http.StatusOK {
		t.Fatalf("duplicate sync expected 200, got %d", second.StatusCode)
	}
	secondOut := decodeGraphSyncResponse(t, second)
	if !secondOut.Duplicate {
		t.Fatalf("expected duplicate=true, got %+v", secondOut)
	}
	if secondOut.GraphDigest != firstOut.GraphDigest {
		t.Fatalf("digest mismatch across duplicate: %q vs %q", secondOut.GraphDigest, firstOut.GraphDigest)
	}
}

func TestDaemon_GraphSync_SyncIDReuseConflict(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	first := postGraphSync(t, addr, graphSyncBody(t, "sync-reuse", validTestExport()))
	if first.StatusCode != http.StatusAccepted {
		t.Fatalf("first sync expected 202, got %d", first.StatusCode)
	}
	first.Body.Close()

	// Same sync ID, different (but still valid) graph content -> 409.
	other := validTestExport()
	other.Nodes[0].Provenance.Producer = "other-producer"
	second := postGraphSync(t, addr, graphSyncBody(t, "sync-reuse", other))
	if second.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 on content mismatch, got %d", second.StatusCode)
	}
	second.Body.Close()
}

func TestDaemon_GraphSync_InvalidSchemaVersion(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	bad := validTestExport()
	bad.SchemaVersion = "not-a-graph"
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-schema", bad))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad schema, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_DanglingEdge(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	bad := validTestExport()
	bad.Edges[0].To = graphcontracts.Ref{Kind: graphcontracts.NodeKnowledge, ID: "missing"}
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-dangling", bad))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for dangling edge, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_DuplicateNode(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	bad := validTestExport()
	bad.Nodes[1].ID = bad.Nodes[0].ID
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-dupn", bad))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate node, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_MissingGraph(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	env := GraphSyncRequest{SyncID: "sync-nograph"}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postGraphSync(t, addr, raw)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing graph, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_TooManyFacts(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	now := time.Now().UTC()
	var nodes []graphcontracts.Node
	for i := 0; i <= maxGraphSyncFacts; i++ {
		nodes = append(nodes, graphcontracts.Node{
			ID:         "n" + string(rune('a'+i%26)) + strconv.Itoa(i),
			Kind:       graphcontracts.NodeSystem,
			CreatedAt:  now,
			Provenance: graphcontracts.Provenance{Producer: "test"},
		})
	}
	bad := validTestExport()
	bad.Nodes = nodes
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-many", bad))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for too many facts, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_ToleratesProducerMetadata(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	raw, err := json.Marshal(validTestExport())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Wrap the graph with producer-specific metadata the portable contract
	// permits (query_sha256). It must be tolerated, not rejected.
	env := GraphSyncRequest{SyncID: "sync-meta", ProjectID: "proj", Graph: raw}
	// Inject query_sha256 by re-marshaling through a map.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m["query_sha256"] = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	withMeta, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	env.Graph = withMeta
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	resp := postGraphSync(t, addr, out)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 tolerating query_sha256, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_OversizedBody(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	// A payload exceeding the 1 MiB request bound must be rejected with 413,
	// matching the cloud plane's /v1/graph/sync contract.
	body := []byte(`{"syncId":"sync-big","projectId":"proj","graph":{"schema_version":"test.graph/v1","generated_at":"2026-01-01T00:00:00Z","nodes":[],"edges":[],"events":[],"padding":"` + strings.Repeat("x", maxRequestBodyBytes) + `"}}`)
	resp := postGraphSync(t, addr, body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized graph sync body, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_ProjectIDRequired(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	raw, err := json.Marshal(validTestExport())
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	env := GraphSyncRequest{SyncID: "sync-noproj", Graph: raw}
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	resp := postGraphSync(t, addr, body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing projectId, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_ScopeMismatch(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	export := validTestExport()
	export.Scope.ProjectID = "other-project"
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-scope", export))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for scope mismatch, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_SensitiveAttribute(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	export := validTestExport()
	export.Nodes[0].Attributes = map[string]string{"model": "gpt-4"}
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-sensitive", export))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for sensitive attribute, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestDaemon_GraphSync_TenantScope(t *testing.T) {
	addr := newGraphSyncTestServer(t)
	export := validTestExport()
	export.Scope.TenantID = "tenant-1"
	resp := postGraphSync(t, addr, graphSyncBody(t, "sync-tenant", export))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for tenant-scoped fact, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
