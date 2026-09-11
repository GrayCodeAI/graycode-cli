package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	MaxGraphSyncBodySize = 1 << 20
	maxGraphResponseSize = 64 << 10
)

var (
	sensitiveGraphAttribute = regexp.MustCompile(`(?i)(content|prompt|secret|credential|password|api[_-]?key|query|reason|url|path|command|provider|model|repository|branch|commit|source|target|message|evidence|element|file|fix)`)
	safeGraphAttribute      = regexp.MustCompile(`(?i)(_sha256|_digest|_count|_tokens?|token_count)$`)
)

// PreparedGraph is a bounded, cloud-safe graph document and its deterministic
// upload identity.
type PreparedGraph struct {
	Graph  json.RawMessage
	SyncID string
	Facts  int
}

type GraphSyncRequest struct {
	SyncID    string          `json:"syncId"`
	ProjectID string          `json:"projectId"`
	SessionID string          `json:"sessionId,omitempty"`
	Graph     json.RawMessage `json:"graph"`
}

type GraphSyncResult struct {
	Accepted    bool   `json:"accepted"`
	Duplicate   bool   `json:"duplicate"`
	GraphDigest string `json:"graphDigest"`
	Facts       int    `json:"facts,omitempty"`
}

// PrepareGraph converts a portable graph to deterministic JSON, hashes values
// behind cloud-sensitive attribute names, and enforces Hawk Cloud's fact caps.
// It does not mutate the caller's graph.
func PrepareGraph(graph any) (PreparedGraph, error) {
	raw, err := json.Marshal(graph)
	if err != nil {
		return PreparedGraph{}, fmt.Errorf("marshal graph: %w", err)
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&document); err != nil {
		return PreparedGraph{}, fmt.Errorf("decode graph: %w", err)
	}

	nodes, err := graphFacts(document, "nodes", 250)
	if err != nil {
		return PreparedGraph{}, err
	}
	edges, err := graphFacts(document, "edges", 500)
	if err != nil {
		return PreparedGraph{}, err
	}
	events, err := graphFacts(document, "events", 500)
	if err != nil {
		return PreparedGraph{}, err
	}
	facts := len(nodes) + len(edges) + len(events)
	if facts > 900 {
		return PreparedGraph{}, fmt.Errorf("graph has %d facts; Hawk Cloud accepts at most 900", facts)
	}

	for _, collection := range [][]any{nodes, edges} {
		for _, fact := range collection {
			item, ok := fact.(map[string]any)
			if !ok {
				return PreparedGraph{}, fmt.Errorf("graph fact must be an object")
			}
			if sanitizeErr := sanitizeGraphAttributes(item); sanitizeErr != nil {
				return PreparedGraph{}, sanitizeErr
			}
		}
	}

	digestInput, err := json.Marshal(document)
	if err != nil {
		return PreparedGraph{}, fmt.Errorf("encode graph digest input: %w", err)
	}
	document["query_sha256"] = sha256Hex(digestInput)
	prepared, err := json.Marshal(document)
	if err != nil {
		return PreparedGraph{}, fmt.Errorf("encode prepared graph: %w", err)
	}
	return PreparedGraph{
		Graph:  prepared,
		SyncID: "graph_" + sha256Hex(prepared),
		Facts:  facts,
	}, nil
}

func graphFacts(document map[string]any, field string, limit int) ([]any, error) {
	value, ok := document[field].([]any)
	if !ok {
		return nil, fmt.Errorf("graph %s must be an array", field)
	}
	if len(value) > limit {
		return nil, fmt.Errorf("graph has %d %s; Hawk Cloud accepts at most %d", len(value), field, limit)
	}
	return value, nil
}

func sanitizeGraphAttributes(fact map[string]any) error {
	value, exists := fact["attributes"]
	if !exists {
		return nil
	}
	attributes, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("graph attributes must be an object")
	}
	sanitized := make(map[string]any, len(attributes))
	for key, value := range attributes {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("graph attribute %q must be a string", key)
		}
		safeKey := key
		safeValue := text
		if sensitiveGraphAttribute.MatchString(key) && !safeGraphAttribute.MatchString(key) {
			safeKey = key + "_sha256"
			safeValue = sha256Hex([]byte(text))
		}
		if _, duplicate := sanitized[safeKey]; duplicate {
			return fmt.Errorf("graph attributes collide after cloud sanitization at %q", safeKey)
		}
		sanitized[safeKey] = safeValue
	}
	fact["attributes"] = sanitized
	return nil
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// SyncGraph uploads an explicitly prepared graph and reports server failures.
// Unlike automatic usage accounting, this method is called by an explicit user
// command, so errors are returned rather than discarded.
func (c *Client) SyncGraph(ctx context.Context, request GraphSyncRequest) (GraphSyncResult, error) {
	var result GraphSyncResult
	if !c.Enabled() {
		return result, fmt.Errorf("hawk cloud is not connected")
	}
	body, err := json.Marshal(request)
	if err != nil {
		return result, fmt.Errorf("marshal graph sync: %w", err)
	}
	if len(body) > MaxGraphSyncBodySize {
		return result, fmt.Errorf("graph sync body is %d bytes; Hawk Cloud accepts at most %d", len(body), MaxGraphSyncBodySize)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint+"/v1/graph/sync",
		bytes.NewReader(body),
	)
	if err != nil {
		return result, fmt.Errorf("create graph sync request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return result, fmt.Errorf("sync graph: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, maxGraphResponseSize)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(limited).Decode(&failure)
		message := strings.TrimSpace(failure.Error)
		if message == "" {
			message = resp.Status
		}
		return result, fmt.Errorf("hawk cloud graph sync: %s", message)
	}
	if err := json.NewDecoder(limited).Decode(&result); err != nil {
		return result, fmt.Errorf("decode graph sync response: %w", err)
	}
	if !result.Accepted {
		return result, fmt.Errorf("hawk cloud did not accept graph sync")
	}
	return result, nil
}
