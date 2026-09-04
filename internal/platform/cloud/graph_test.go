package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/graycode-cli/internal/contracts/graph"
	"github.com/GrayCodeAI/graycode-cli/internal/executiongraph"
)

func TestPrepareGraphSanitizesSensitiveAttributesDeterministically(t *testing.T) {
	graph := executiongraph.Export{
		SchemaVersion: executiongraph.SchemaVersion,
		GeneratedAt:   time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC),
		Nodes: []graphcontracts.Node{{
			ID:        "graycode/session/session-0123456789",
			Kind:      graphcontracts.NodeExecution,
			CreatedAt: time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC),
			Provenance: graphcontracts.Provenance{
				Producer: "graycode",
			},
			Attributes: map[string]string{
				"entity_type": "graycode_session",
				"provider":    "private-provider",
				"model":       "private-model",
			},
		}},
		Edges:  []graphcontracts.Edge{},
		Events: []graphcontracts.Event{},
	}

	first, err := PrepareGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PrepareGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	if first.SyncID != second.SyncID || string(first.Graph) != string(second.Graph) {
		t.Fatal("prepared graph is not deterministic")
	}
	if strings.Contains(string(first.Graph), "private-provider") ||
		strings.Contains(string(first.Graph), "private-model") {
		t.Fatal("prepared graph leaked sensitive attribute values")
	}

	var document map[string]any
	if err := json.Unmarshal(first.Graph, &document); err != nil {
		t.Fatal(err)
	}
	node := document["nodes"].([]any)[0].(map[string]any)
	attributes := node["attributes"].(map[string]any)
	if _, ok := attributes["provider_sha256"]; !ok {
		t.Fatal("provider was not converted to provider_sha256")
	}
	if _, ok := attributes["model_sha256"]; !ok {
		t.Fatal("model was not converted to model_sha256")
	}
	if first.Facts != 1 {
		t.Fatalf("facts = %d, want 1", first.Facts)
	}
}

func TestPrepareGraphRejectsCloudLimits(t *testing.T) {
	graph := map[string]any{
		"schema_version": "graycode.graph/v1",
		"generated_at":   "2026-07-25T12:00:00Z",
		"nodes":          make([]map[string]any, 251),
		"edges":          []any{},
		"events":         []any{},
	}
	if _, err := PrepareGraph(graph); err == nil || !strings.Contains(err.Error(), "250") {
		t.Fatalf("error = %v, want node limit error", err)
	}
}

func TestSyncGraphUsesDeviceAuthAndDecodesResult(t *testing.T) {
	var gotAuth, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true,"duplicate":false,"graphDigest":"abc","facts":1}`))
	}))
	defer server.Close()

	client := New(Config{Endpoint: server.URL, DeviceToken: "hwc_test"})
	result, err := client.SyncGraph(context.Background(), GraphSyncRequest{
		SyncID:    "graph_0123456789abcdef",
		ProjectID: "project_0123456789",
		Graph:     json.RawMessage(`{"schema_version":"graycode.graph/v1","generated_at":"2026-07-25T12:00:00Z","nodes":[],"edges":[],"events":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/graph/sync" || gotAuth != "Bearer hwc_test" {
		t.Fatalf("path/auth = %q/%q", gotPath, gotAuth)
	}
	if !result.Accepted || result.Facts != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestSyncGraphReportsBoundedServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Graph sync ID already has different content"}`))
	}))
	defer server.Close()

	_, err := New(Config{Endpoint: server.URL, DeviceToken: "hwc_test"}).SyncGraph(
		context.Background(),
		GraphSyncRequest{
			SyncID:    "graph_0123456789abcdef",
			ProjectID: "project_0123456789",
			Graph:     json.RawMessage(`{}`),
		},
	)
	if err == nil || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("error = %v", err)
	}
}
