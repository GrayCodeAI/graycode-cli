package daemon

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// GET /v1/projects/:projectId/graph lets a producer read back the portable
// facts it ingested via POST /v1/graph/sync, mirroring the cloud plane's read
// projection so a producer can verify what was accepted on either surface.
// The local daemon retains whole graph documents (not normalized fact rows),
// so it returns a single page per fact type and no pagination cursor.

const (
	defaultGraphReadLimit  = 100
	maxGraphReadLimit      = 250
	graphReadSchemaVersion = "graycode-local.graph/v1"
)

// GraphReadResponse is the JSON response for GET /v1/projects/:projectId/graph.
type GraphReadResponse struct {
	Graph      graphReadProjection `json:"graph"`
	Limits     graphReadLimits     `json:"limits"`
	Pagination graphReadPagination `json:"pagination"`
}

type graphReadProjection struct {
	SchemaVersion string            `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	Scope         map[string]string `json:"scope"`
	Nodes         []json.RawMessage `json:"nodes"`
	Edges         []json.RawMessage `json:"edges"`
	Events        []json.RawMessage `json:"events"`
}

type graphReadLimits struct {
	PerFactType int `json:"perFactType"`
}

type graphReadPagination struct {
	NextCursor any `json:"next_cursor"`
}

// graphSyncDocument is the stored producer payload shape (portable facts are
// passed through verbatim, matching the cloud read projection).
type graphSyncDocument struct {
	Nodes  []json.RawMessage `json:"nodes"`
	Edges  []json.RawMessage `json:"edges"`
	Events []json.RawMessage `json:"events"`
}

// handleGraphRead handles GET /v1/projects/:projectId/graph.
func (s *Server) handleGraphRead(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "projectId is required"})
		return
	}
	if len(projectID) > maxGraphSyncProjectLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "projectId is too long"})
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	if len(sessionID) > maxGraphSyncSessionLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId is too long"})
		return
	}
	limit := defaultGraphReadLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
			return
		}
		if n > maxGraphReadLimit {
			n = maxGraphReadLimit
		}
		limit = n
	}

	records, err := s.graphLedger.List(r.Context(), projectID, sessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "graph ledger read failed"})
		return
	}

	nodes := make([]json.RawMessage, 0, limit)
	edges := make([]json.RawMessage, 0, limit)
	events := make([]json.RawMessage, 0, limit)
	for _, rec := range records {
		var doc graphSyncDocument
		if err := json.Unmarshal([]byte(rec.GraphJSON), &doc); err != nil {
			continue
		}
		if len(nodes) < limit {
			nodes = append(nodes, doc.Nodes...)
			if len(nodes) > limit {
				nodes = nodes[:limit]
			}
		}
		if len(edges) < limit {
			edges = append(edges, doc.Edges...)
			if len(edges) > limit {
				edges = edges[:limit]
			}
		}
		if len(events) < limit {
			events = append(events, doc.Events...)
			if len(events) > limit {
				events = events[:limit]
			}
		}
	}

	writeJSON(w, http.StatusOK, GraphReadResponse{
		Graph: graphReadProjection{
			SchemaVersion: graphReadSchemaVersion,
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			Scope:         map[string]string{"project_id": projectID},
			Nodes:         nodes,
			Edges:         edges,
			Events:        events,
		},
		Limits:     graphReadLimits{PerFactType: limit},
		Pagination: graphReadPagination{NextCursor: nil},
	})
}
