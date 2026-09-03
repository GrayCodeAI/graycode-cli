package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/graycode-cli/internal/testutil"
)

// TestDaemon_GraphSync_ContractMatrix locks the /v1/graph/sync parity contract
// on the daemon surface: for every scenario the shared contract defines, the
// local daemon must return exactly the documented status. The cloud worker
// enforces the identical matrix in graycode-platform test/graph.test.ts, so a
// producer targeting either surface observes the same accept/duplicate/
// conflict/invalid behavior.
func TestDaemon_GraphSync_ContractMatrix(t *testing.T) {
	matrix := []struct {
		name string
		run  func(t *testing.T, addr string)
	}{
		{"accepted_202", func(t *testing.T, addr string) {
			assertSyncStatus(t, addr, graphSyncBody(t, "m-accepted", validTestExport()), http.StatusAccepted)
		}},
		{"duplicate_200", func(t *testing.T, addr string) {
			body := graphSyncBody(t, "m-dup", validTestExport())
			assertSyncStatus(t, addr, body, http.StatusAccepted)
			assertSyncStatus(t, addr, body, http.StatusOK)
		}},
		{"missing_project_id_400", func(t *testing.T, addr string) {
			raw, _ := json.Marshal(validTestExport())
			env := GraphSyncRequest{SyncID: "m-noproj", Graph: raw}
			body, _ := json.Marshal(env)
			assertSyncStatus(t, addr, body, http.StatusBadRequest)
		}},
		{"missing_graph_400", func(t *testing.T, addr string) {
			body := []byte(`{"syncId":"m-nograph","projectId":"proj"}`)
			assertSyncStatus(t, addr, body, http.StatusBadRequest)
		}},
		{"invalid_schema_400", func(t *testing.T, addr string) {
			export := validTestExport()
			export.SchemaVersion = "not-a-graph"
			assertSyncStatus(t, addr, graphSyncBody(t, "m-schema", export), http.StatusBadRequest)
		}},
		{"dangling_edge_400", func(t *testing.T, addr string) {
			export := validTestExport()
			export.Edges[0].To.ID = "missing-node"
			assertSyncStatus(t, addr, graphSyncBody(t, "m-dangle", export), http.StatusBadRequest)
		}},
		{"duplicate_node_400", func(t *testing.T, addr string) {
			export := validTestExport()
			export.Nodes[1].ID = export.Nodes[0].ID
			assertSyncStatus(t, addr, graphSyncBody(t, "m-dupnode", export), http.StatusBadRequest)
		}},
		{"too_many_facts_400", func(t *testing.T, addr string) {
			export := validTestExport()
			for i := 0; i < 900; i++ {
				export.Nodes = append(export.Nodes, graphcontracts.Node{
					ID: "extra-" + strconv.Itoa(i), Kind: graphcontracts.NodeSystem,
					CreatedAt: time.Now().UTC(), Provenance: graphcontracts.Provenance{Producer: "test"},
				})
			}
			assertSyncStatus(t, addr, graphSyncBody(t, "m-many", export), http.StatusBadRequest)
		}},
		{"scope_mismatch_409", func(t *testing.T, addr string) {
			export := validTestExport()
			export.Scope.ProjectID = "other-project"
			assertSyncStatus(t, addr, graphSyncBody(t, "m-scope", export), http.StatusConflict)
		}},
		{"sync_id_reuse_different_content_409", func(t *testing.T, addr string) {
			assertSyncStatus(t, addr, graphSyncBody(t, "m-reuse", validTestExport()), http.StatusAccepted)
			export := validTestExport()
			export.Nodes[0].Provenance.Producer = "other-producer"
			assertSyncStatus(t, addr, graphSyncBody(t, "m-reuse", export), http.StatusConflict)
		}},
		{"sensitive_attribute_400", func(t *testing.T, addr string) {
			export := validTestExport()
			export.Nodes[0].Attributes = map[string]string{"model": "gpt-4"}
			assertSyncStatus(t, addr, graphSyncBody(t, "m-sensitive", export), http.StatusBadRequest)
		}},
		{"tenant_scope_400", func(t *testing.T, addr string) {
			export := validTestExport()
			export.Scope.TenantID = "tenant-1"
			assertSyncStatus(t, addr, graphSyncBody(t, "m-tenant", export), http.StatusBadRequest)
		}},
		{"oversized_body_413", func(t *testing.T, addr string) {
			body := []byte(`{"syncId":"m-big","projectId":"proj","graph":{"schema_version":"test.graph/v1","generated_at":"2026-01-01T00:00:00Z","nodes":[],"edges":[],"events":[],"padding":"` + strings.Repeat("x", maxRequestBodyBytes) + `"}}`)
			assertSyncStatus(t, addr, body, http.StatusRequestEntityTooLarge)
		}},
		{"unauthorized_401", func(t *testing.T, addr string) {
			srv := New(Config{Port: 0, Host: testutil.LoopbackHost, APIKey: "secret"}, nil)
			authedAddr := startTestDaemon(t, srv)
			defer srv.Stop(context.Background())
			assertSyncStatus(t, authedAddr, graphSyncBody(t, "m-auth", validTestExport()), http.StatusUnauthorized)
		}},
	}

	for _, tc := range matrix {
		t.Run(tc.name, func(t *testing.T) {
			addr := newGraphSyncTestServer(t)
			tc.run(t, addr)
		})
	}
}

func assertSyncStatus(t *testing.T, addr string, body []byte, want int) {
	t.Helper()
	resp, err := http.Post("http://"+addr+"/v1/graph/sync", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/graph/sync failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("expected status %d, got %d", want, resp.StatusCode)
	}
}
