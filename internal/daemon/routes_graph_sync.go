package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/hawk/internal/executiongraph"
)

// POST /v1/graph/sync lets a producer (a future ecosystem repo) push portable
// `*.graph/v1` facts into Hawk. It mirrors the cloud plane's /v1/graph/sync
// contract so a producer can target either surface with the same payload.
// The daemon is a localhost consumer surface: it validates the graph, rejects
// malformed or non-portable facts, and acknowledges with an idempotency digest.
// Persistence is an in-memory ledger (matching the daemon's in-memory session
// architecture); durable retention is the cloud plane's job.
const (
	maxGraphSyncNodes  = 250
	maxGraphSyncEdges  = 500
	maxGraphSyncEvents = 500
	maxGraphSyncFacts  = 900

	maxGraphSyncIDLen      = 256
	maxGraphSyncProjectLen = 128
	maxGraphSyncSessionLen = 128
)

// graphSchemaVersionPattern accepts any `<name>.graph/v1` schema version,
// matching the cloud plane's portable-graph contract.
var graphSchemaVersionPattern = regexp.MustCompile(`^[a-z0-9-]+\.graph/v1$`)

// graphSensitiveAttribute and graphSafeSensitiveAttribute mirror the cloud
// plane's sensitive-attribute policy: a node/edge attribute key that names
// sensitive content is rejected unless it is an explicit digest/count (or
// sast_source). Producers that want those values to reach the cloud must
// hash them behind a `_sha256` key (as PrepareGraph does).
var (
	graphSensitiveAttribute     = regexp.MustCompile(`(?i)(?:content|prompt|secret|credential|password|api[_-]?key|query|reason|url|path|command|provider|model|repository|branch|commit|source|target|message|evidence|element|file|fix)`)
	graphSafeSensitiveAttribute = regexp.MustCompile(`(?:_sha256|_digest|_count|_tokens?|token_count)$`)
)

// GraphSyncRequest is the JSON body for POST /v1/graph/sync. The graph is
// captured as raw JSON so producer-specific metadata (e.g. query_sha256) is
// tolerated while the core portable-graph shape is still validated.
type GraphSyncRequest struct {
	SyncID    string          `json:"syncId"`
	ProjectID string          `json:"projectId,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
	Graph     json.RawMessage `json:"graph"`
}

// GraphSyncResponse is the JSON response from POST /v1/graph/sync.
type GraphSyncResponse struct {
	Accepted    bool   `json:"accepted"`
	Duplicate   bool   `json:"duplicate"`
	GraphDigest string `json:"graphDigest"`
	Facts       int    `json:"facts,omitempty"`
}

// decodeGraphSyncBody decodes the request body like decodeJSONBody, but maps
// an oversized body to 413 so /v1/graph/sync matches the cloud plane's
// oversized-body contract. Real main's shared decodeJSONBody returns 400 for
// every decode error, so the sync surface needs its own limit handling.
func decodeGraphSyncBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		}
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request body must contain a single JSON object"})
		return false
	}
	return true
}

// handleGraphSync handles POST /v1/graph/sync.
func (s *Server) handleGraphSync(w http.ResponseWriter, r *http.Request) {
	var req GraphSyncRequest
	if !decodeGraphSyncBody(w, r, &req) {
		return
	}

	syncID := strings.TrimSpace(req.SyncID)
	if syncID == "" || len(syncID) > maxGraphSyncIDLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "syncId is required"})
		return
	}
	if req.ProjectID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "projectId is required"})
		return
	}
	if len(req.ProjectID) > maxGraphSyncProjectLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "projectId is too long"})
		return
	}
	if len(req.SessionID) > maxGraphSyncSessionLen {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionId is too long"})
		return
	}
	if len(req.Graph) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "graph is required"})
		return
	}

	var export executiongraph.Export
	if err := json.Unmarshal(req.Graph, &export); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid graph: " + err.Error()})
		return
	}
	if err := validateGraphExport(export); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !graphScopeMatchesProject(export, req.ProjectID) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "graph scope does not match project"})
		return
	}
	if graphHasUnsafeCloudData(export) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "graph contains non-portable or sensitive metadata"})
		return
	}

	digest := graphDigest(export)
	facts := len(export.Nodes) + len(export.Edges) + len(export.Events)

	rec := GraphSyncRecord{
		SyncID:        syncID,
		ProjectID:     req.ProjectID,
		SessionID:     req.SessionID,
		SchemaVersion: export.SchemaVersion,
		Digest:        digest,
		Facts:         facts,
		GraphJSON:     string(req.Graph),
		ReceivedAt:    time.Now().UTC(),
	}
	inserted, err := s.graphLedger.InsertIfAbsent(r.Context(), rec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to persist graph sync"})
		return
	}
	if !inserted {
		existing, ok, err := s.graphLedger.Get(r.Context(), syncID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read graph sync"})
			return
		}
		if !ok || existing.Digest != digest {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "graph sync ID already has different content"})
			return
		}
		writeJSON(w, http.StatusOK, GraphSyncResponse{Accepted: true, Duplicate: true, GraphDigest: digest})
		return
	}

	writeJSON(w, http.StatusAccepted, GraphSyncResponse{
		Accepted:    true,
		Duplicate:   false,
		GraphDigest: digest,
		Facts:       facts,
	})
}

// validateGraphExport checks a portable graph against the shared `*.graph/v1`
// contract: schema version, fact-count bounds, per-fact validity (reusing
// eagle/graph), unique identities, and self-contained topology (edges and
// events may only reference nodes present in the same export).
func validateGraphExport(export executiongraph.Export) error {
	if !graphSchemaVersionPattern.MatchString(export.SchemaVersion) {
		return fmt.Errorf("graph: unsupported schema_version %q", export.SchemaVersion)
	}
	if export.GeneratedAt.IsZero() {
		return fmt.Errorf("graph: generated_at is required")
	}
	if len(export.Nodes) > maxGraphSyncNodes {
		return fmt.Errorf("graph: too many nodes (%d > %d)", len(export.Nodes), maxGraphSyncNodes)
	}
	if len(export.Edges) > maxGraphSyncEdges {
		return fmt.Errorf("graph: too many edges (%d > %d)", len(export.Edges), maxGraphSyncEdges)
	}
	if len(export.Events) > maxGraphSyncEvents {
		return fmt.Errorf("graph: too many events (%d > %d)", len(export.Events), maxGraphSyncEvents)
	}
	if len(export.Nodes)+len(export.Edges)+len(export.Events) > maxGraphSyncFacts {
		return fmt.Errorf("graph: exceeds the %d fact limit", maxGraphSyncFacts)
	}

	nodeIDs := make(map[string]struct{}, len(export.Nodes))
	for i, n := range export.Nodes {
		if err := n.Validate(); err != nil {
			return fmt.Errorf("graph: node[%d]: %w", i, err)
		}
		if _, dup := nodeIDs[n.ID]; dup {
			return fmt.Errorf("graph: duplicate node %q", n.ID)
		}
		nodeIDs[n.ID] = struct{}{}
	}

	edgeIDs := make(map[string]struct{}, len(export.Edges))
	for i, e := range export.Edges {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("graph: edge[%d]: %w", i, err)
		}
		if _, dup := edgeIDs[e.ID]; dup {
			return fmt.Errorf("graph: duplicate edge %q", e.ID)
		}
		edgeIDs[e.ID] = struct{}{}
		if _, ok := nodeIDs[e.From.ID]; !ok {
			return fmt.Errorf("graph: dangling edge %q references unknown node %q", e.ID, e.From.ID)
		}
		if _, ok := nodeIDs[e.To.ID]; !ok {
			return fmt.Errorf("graph: dangling edge %q references unknown node %q", e.ID, e.To.ID)
		}
	}

	eventIDs := make(map[string]struct{}, len(export.Events))
	for i, ev := range export.Events {
		if err := ev.Validate(); err != nil {
			return fmt.Errorf("graph: event[%d]: %w", i, err)
		}
		if _, dup := eventIDs[ev.ID]; dup {
			return fmt.Errorf("graph: duplicate event %q", ev.ID)
		}
		eventIDs[ev.ID] = struct{}{}
		if _, ok := nodeIDs[ev.Subject.ID]; !ok {
			return fmt.Errorf("graph: dangling event %q references unknown node %q", ev.ID, ev.Subject.ID)
		}
	}
	return nil
}

// graphScopeMatchesProject reports whether every fact scope's project_id, when
// present, equals projectID — mirroring the cloud plane's scope check (409 on
// mismatch).
func graphScopeMatchesProject(export executiongraph.Export, projectID string) bool {
	for _, sc := range graphScopes(export) {
		if sc.ProjectID != "" && sc.ProjectID != projectID {
			return false
		}
	}
	return true
}

// graphHasUnsafeCloudData reports whether the export carries facts the cloud
// plane rejects as non-portable or sensitive: a tenant-scoped fact, or a
// node/edge attribute whose key names sensitive content. Mirrors the cloud
// plane's graphHasUnsafeCloudData.
func graphHasUnsafeCloudData(export executiongraph.Export) bool {
	for _, sc := range graphScopes(export) {
		if sc.TenantID != "" {
			return true
		}
	}
	for _, n := range export.Nodes {
		if hasUnsafeAttributes(n.Attributes) {
			return true
		}
	}
	for _, e := range export.Edges {
		if hasUnsafeAttributes(e.Attributes) {
			return true
		}
	}
	return false
}

// graphScopes returns the export-level and per-fact scopes, in the same order
// the cloud plane inspects them.
func graphScopes(export executiongraph.Export) []graphcontracts.Scope {
	scopes := make([]graphcontracts.Scope, 0, 1+len(export.Nodes)+len(export.Edges)+len(export.Events))
	scopes = append(scopes, export.Scope)
	for _, n := range export.Nodes {
		scopes = append(scopes, n.Scope)
	}
	for _, e := range export.Edges {
		scopes = append(scopes, e.Scope)
	}
	for _, ev := range export.Events {
		scopes = append(scopes, ev.Scope)
	}
	return scopes
}

// hasUnsafeAttributes reports whether any attribute key names sensitive content
// under the shared sensitive-attribute policy.
func hasUnsafeAttributes(attrs map[string]string) bool {
	for key := range attrs {
		if key == "sast_source" {
			continue
		}
		if graphSensitiveAttribute.MatchString(key) && !graphSafeSensitiveAttribute.MatchString(key) {
			return true
		}
	}
	return false
}

// graphDigest returns a deterministic SHA-256 over the normalized export.
// encoding/json sorts map keys, so Marshal output is stable across runs.
func graphDigest(export executiongraph.Export) string {
	raw, err := json.Marshal(export)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
