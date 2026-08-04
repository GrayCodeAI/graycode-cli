package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/hawk-core-contracts/graph"
	"github.com/GrayCodeAI/hawk/internal/graphjournal"
	"github.com/GrayCodeAI/hawk/internal/hawkerr"
	yaad "github.com/GrayCodeAI/yaad"
	yaadEngine "github.com/GrayCodeAI/yaad/engine"
	yaadGraph "github.com/GrayCodeAI/yaad/graph"
	"github.com/GrayCodeAI/yaad/portablegraph"
	"github.com/GrayCodeAI/yaad/storage"
)

// YaadBridge connects hawk's memory system to the yaad memory graph.
// If yaad is not initialized (missing DB), operations return a BridgeError
// and log a warning on first access.
type YaadBridge struct {
	engine   *yaadEngine.Engine
	store    *storage.Store
	mu       sync.Mutex
	ready    bool
	warnOnce sync.Once

	graphSessionID string
	graphScope     graphcontracts.Scope
}

// NewYaadBridge initializes a bridge to yaad's SQLite store at ~/.yaad/data/yaad.db.
// Returns a bridge that silently no-ops if initialization fails.
func NewYaadBridge() *YaadBridge {
	b := &YaadBridge{}
	b.init()
	return b
}

func (b *YaadBridge) init() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dbDir := filepath.Join(home, ".yaad", "data")
	if mkErr := os.MkdirAll(dbDir, 0o700); mkErr != nil {
		return
	}
	dbPath := filepath.Join(dbDir, "yaad.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		return
	}

	g := yaadGraph.New(store, store.DB())
	eng := yaadEngine.New(store, g)

	b.store = store
	b.engine = eng
	b.ready = true
}

// Ready reports whether the yaad bridge is initialized and usable.
func (b *YaadBridge) Ready() bool {
	return b.ready
}

// IsReady is a public alias for Ready, exported for external consumers
// that need to check bridge status before batching operations.
func (b *YaadBridge) IsReady() bool {
	return b.ready
}

// ConfigureGraphObservation binds future successful recalls to a persisted
// Hawk session. Empty session IDs disable capture.
func (b *YaadBridge) ConfigureGraphObservation(sessionID string, scope graphcontracts.Scope) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.graphSessionID = strings.TrimSpace(sessionID)
	b.graphScope = scope
	b.mu.Unlock()
}

// notReadyError logs a warning once and returns a structured BridgeError.
func (b *YaadBridge) notReadyError(op string) error {
	b.warnOnce.Do(func() {
		log.Println("[hawk/memory] WARNING: yaad bridge is not initialized; memory operations will be skipped. Ensure ~/.yaad/data/ is accessible.")
	})
	return &hawkerr.BridgeError{
		Bridge: "yaad",
		Op:     op,
		Reason: "bridge not initialized",
	}
}

// Remember stores content into yaad's memory graph under the given category.
// Category maps to yaad's node type (e.g., "convention", "decision", "bug", "preference").
// Returns a BridgeError if yaad is not initialized.
func (b *YaadBridge) Remember(content, category string) error {
	return b.RememberWithContext(context.Background(), content, category)
}

// RememberWithContext is the context-aware version of Remember.
func (b *YaadBridge) RememberWithContext(ctx context.Context, content, category string) error {
	if !b.ready {
		return b.notReadyError("Remember")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nodeType := category
	if !yaadEngine.IsValidNodeType(nodeType) {
		nodeType = "convention"
	}

	_, err := b.engine.Remember(ctx, yaadEngine.RememberInput{
		Type:    nodeType,
		Content: content,
		Scope:   "project",
	})
	return err
}

// Recall searches yaad's memory graph and returns formatted context that fits
// within the specified token budget. Returns a BridgeError if yaad is not initialized.
func (b *YaadBridge) Recall(query string, tokenBudget int) (string, error) {
	return b.RecallWithContext(context.Background(), query, tokenBudget)
}

// RecallWithContext is the context-aware version of Recall.
func (b *YaadBridge) RecallWithContext(ctx context.Context, query string, tokenBudget int) (string, error) {
	if !b.ready {
		return "", b.notReadyError("Recall")
	}

	result, err := b.recallResultWithContext(ctx, yaadEngine.RecallOpts{
		Query:  query,
		Budget: tokenBudget,
		Limit:  10,
		Depth:  2,
	})
	if err != nil {
		return "", err
	}
	if result == nil || len(result.Nodes) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for i, node := range result.Nodes {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("[%s] %s", node.Type, node.Content))
	}
	return sb.String(), nil
}

func (b *YaadBridge) recallResultWithContext(
	ctx context.Context,
	opts yaadEngine.RecallOpts,
) (*yaadEngine.RecallResult, error) {
	if !b.ready {
		return nil, b.notReadyError("Recall")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	result, err := b.engine.Recall(ctx, opts)
	if err != nil {
		return nil, err
	}
	if result != nil && len(result.Nodes) > 0 {
		b.recordContextGraph(opts.Query, result)
	}
	return result, nil
}

// listNodes is the memory package's read boundary for Yaad node queries.
// Callers stay independent of the storage lifecycle and synchronization.
func (b *YaadBridge) listNodes(ctx context.Context, filter storage.NodeFilter) ([]*storage.Node, error) {
	if !b.ready {
		return nil, b.notReadyError("ListNodes")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.store.ListNodes(ctx, filter)
}

func (b *YaadBridge) listPinnedNodes(ctx context.Context, limit int) ([]*storage.Node, error) {
	pinned := true
	return b.listNodes(ctx, storage.NodeFilter{Pinned: &pinned, Limit: limit})
}

func (b *YaadBridge) listNodesByType(ctx context.Context, nodeType string, minConfidence float64, limit int) ([]*storage.Node, error) {
	return b.listNodes(ctx, storage.NodeFilter{
		Type:          nodeType,
		MinConfidence: minConfidence,
		Limit:         limit,
	})
}

func (b *YaadBridge) listNodesByScope(ctx context.Context, nodeType, scope string, minConfidence float64, limit int) ([]*storage.Node, error) {
	return b.listNodes(ctx, storage.NodeFilter{
		Type:          nodeType,
		Scope:         scope,
		MinConfidence: minConfidence,
		Limit:         limit,
	})
}

func (b *YaadBridge) adjustNodeConfidence(ctx context.Context, id string, delta float64, skipPinned bool) error {
	if !b.ready {
		return b.notReadyError("AdjustNodeConfidence")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	node, err := b.store.GetNode(ctx, id)
	if err != nil || node == nil {
		return err
	}
	if skipPinned && node.Pinned {
		return nil
	}

	node.Confidence += delta
	if node.Confidence > 1.0 {
		node.Confidence = 1.0
	}
	if node.Confidence < 0.1 {
		node.Confidence = 0.1
	}
	return b.store.UpdateNode(ctx, node)
}

func (b *YaadBridge) recallBudget(ctx context.Context, query string, budget, limit, depth int) (*yaadEngine.RecallResult, error) {
	return b.recallResultWithContext(ctx, yaadEngine.RecallOpts{
		Query:  query,
		Budget: budget,
		Limit:  limit,
		Depth:  depth,
	})
}

func (b *YaadBridge) recallProject(ctx context.Context, query, project string, budget, limit, depth int) (*yaadEngine.RecallResult, error) {
	return b.recallResultWithContext(ctx, yaadEngine.RecallOpts{
		Query:   query,
		Budget:  budget,
		Limit:   limit,
		Depth:   depth,
		Project: project,
	})
}

func (b *YaadBridge) rememberGlobal(ctx context.Context, content, nodeType string) error {
	if !b.ready {
		return b.notReadyError("RememberGlobal")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if !yaadEngine.IsValidNodeType(nodeType) {
		nodeType = "preference"
	}
	_, err := b.engine.Remember(ctx, yaadEngine.RememberInput{
		Type:    nodeType,
		Content: content,
		Scope:   "global",
		Project: "__global__",
	})
	return err
}

func (b *YaadBridge) searchNodes(ctx context.Context, query string, limit int) ([]*storage.Node, error) {
	if !b.ready {
		return nil, b.notReadyError("SearchNodes")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.store.SearchNodes(ctx, query, limit)
}

func (b *YaadBridge) createTouchEdge(ctx context.Context, fromID, toID string) error {
	if !b.ready {
		return b.notReadyError("CreateEdge")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.store.CreateEdge(ctx, &storage.Edge{
		FromID: fromID,
		ToID:   toID,
		Type:   "touches",
		Weight: 0.8,
	})
}

func (b *YaadBridge) getOrCreateFileAnchor(ctx context.Context, path string) (*storage.Node, error) {
	if !b.ready {
		return nil, b.notReadyError("GetOrCreateFileAnchor")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	key := "file:" + path
	if node, err := b.store.GetNodeByKey(ctx, key, ""); err == nil && node != nil {
		return node, nil
	}

	node := &storage.Node{
		Type:       "file",
		Content:    "File: " + filepath.Base(path) + " (" + path + ")",
		Scope:      "project",
		Tier:       2,
		Confidence: 0.9,
		Key:        key,
	}
	if err := b.store.CreateNode(ctx, node); err != nil {
		return nil, err
	}
	return node, nil
}

func (b *YaadBridge) recordContextGraph(query string, result *yaadEngine.RecallResult) {
	if b.graphSessionID == "" || result == nil || len(result.Nodes) == 0 {
		return
	}
	projection, err := portablegraph.Build(portablegraph.Input{
		Nodes:           result.Nodes,
		Edges:           result.Edges,
		Query:           query,
		GeneratedAt:     time.Now(),
		Scope:           b.graphScope,
		ProducerVersion: yaad.Version,
	})
	if err != nil {
		log.Printf("[hawk/memory] yaad context graph projection failed: %v", err)
		return
	}
	if err := graphjournal.AppendContextGraph(
		b.graphSessionID,
		"yaad",
		projection.QuerySHA256,
		projection.Nodes,
		projection.Edges,
		projection.Events,
		projection.GeneratedAt,
	); err != nil {
		log.Printf("[hawk/memory] yaad context graph observation failed: %v", err)
	}
}

func (b *YaadBridge) recordSelectedContext(label string, nodes []*storage.Node) {
	if b == nil || b.graphSessionID == "" || len(nodes) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node != nil && strings.TrimSpace(node.ID) != "" {
			ids = append(ids, node.ID)
		}
	}
	edges, err := b.store.GetEdgesBetween(context.Background(), ids)
	if err != nil {
		log.Printf("[hawk/memory] yaad selected context edge lookup failed: %v", err)
		edges = nil
	}
	b.recordContextGraph(label, &yaadEngine.RecallResult{Nodes: nodes, Edges: edges})
}

// InitCodeIndex creates the code index tables in yaad's store.
// Safe to call multiple times. Returns a BridgeError if yaad is not initialized.
func (b *YaadBridge) InitCodeIndex() error {
	if !b.ready {
		return b.notReadyError("InitCodeIndex")
	}
	return b.store.CreateCodeIndex(context.Background())
}

// IndexCodeChunk stores a code chunk in the yaad code index.
func (b *YaadBridge) IndexCodeChunk(path, content, symbol, lang string, start, end, tokens int, hash string) error {
	if !b.ready {
		return b.notReadyError("IndexCodeChunk")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	id := fmt.Sprintf("%s:%d-%d", path, start, end)
	return b.store.UpsertCodeChunk(context.Background(), &storage.CodeChunkRecord{
		ID:        id,
		Path:      path,
		StartLine: start,
		EndLine:   end,
		Content:   content,
		Symbol:    symbol,
		Language:  lang,
		Tokens:    tokens,
		FileHash:  hash,
	})
}

// CodeSearchResult represents a code chunk returned by a search.
type CodeSearchResult struct {
	Path      string
	StartLine int
	EndLine   int
	Content   string
	Symbol    string
	Score     float64
}

// SearchCode performs full-text search over indexed code chunks.
func (b *YaadBridge) SearchCode(query string, limit int) ([]CodeSearchResult, error) {
	if !b.ready {
		return nil, b.notReadyError("SearchCode")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	records, err := b.store.SearchCodeChunksFTS(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}
	b.recordCodeContext(query, records)

	results := make([]CodeSearchResult, len(records))
	for i, r := range records {
		results[i] = CodeSearchResult{
			Path:      r.Path,
			StartLine: r.StartLine,
			EndLine:   r.EndLine,
			Content:   r.Content,
			Symbol:    r.Symbol,
		}
	}
	return results, nil
}

func (b *YaadBridge) recordCodeContext(query string, records []*storage.CodeChunkRecord) {
	if b.graphSessionID == "" || len(records) == 0 {
		return
	}
	occurredAt := time.Now().UTC()
	nodes := make([]graphcontracts.Node, 0, len(records))
	events := make([]graphcontracts.Event, 0, len(records))
	querySHA256 := bridgeDigest(query)
	for _, record := range records {
		if record == nil {
			continue
		}
		sourceID := bridgeDigest(record.ID)
		ref := graphcontracts.Ref{
			Kind: graphcontracts.NodeKnowledge,
			ID:   "hawk/code-chunk/" + sourceID,
		}
		node := graphcontracts.Node{
			ID:        ref.ID,
			Kind:      ref.Kind,
			Scope:     b.graphScope,
			CreatedAt: occurredAt,
			Provenance: graphcontracts.Provenance{
				Producer: "hawk",
				SourceID: sourceID,
				Evidence: []graphcontracts.ArtifactRef{{URI: "hawk://code-index/" + sourceID}},
			},
			Attributes: map[string]string{
				"entity_type":         "code_chunk",
				"data_classification": "metadata_only",
				"path_sha256":         bridgeDigest(record.Path),
				"content_sha256":      bridgeDigest(record.Content),
				"symbol_sha256":       bridgeDigest(record.Symbol),
				"language":            strings.TrimSpace(record.Language),
				"start_line":          strconv.Itoa(record.StartLine),
				"end_line":            strconv.Itoa(record.EndLine),
				"tokens":              strconv.Itoa(record.Tokens),
				"temporal_precision":  "projection_time",
			},
		}
		nodes = append(nodes, node)
		events = append(events, graphcontracts.Event{
			ID:             "hawk/event/code-chunk/" + sourceID + "/observed/" + querySHA256,
			Type:           graphcontracts.EventObserved,
			Subject:        ref,
			Scope:          b.graphScope,
			OccurredAt:     occurredAt,
			CorrelationID:  querySHA256,
			IdempotencyKey: "hawk/code-chunk/" + sourceID + "/observed/" + querySHA256,
			Provenance:     node.Provenance,
		})
	}
	if err := graphjournal.AppendContextGraph(
		b.graphSessionID,
		"hawk-code-index",
		querySHA256,
		nodes,
		nil,
		events,
		occurredAt,
	); err != nil {
		log.Printf("[hawk/memory] code context graph observation failed: %v", err)
	}
}

// GetFileHash returns the stored hash for a file path, or empty string if not indexed.
func (b *YaadBridge) GetFileHash(path string) (string, error) {
	if !b.ready {
		return "", b.notReadyError("GetFileHash")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.GetFileHash(context.Background(), path)
}

// ClearFileChunks removes all indexed chunks for a given file path.
func (b *YaadBridge) ClearFileChunks(path string) error {
	if !b.ready {
		return b.notReadyError("ClearFileChunks")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.DeleteChunksByPath(context.Background(), path)
}

// ListIndexedPaths returns all file paths that have indexed code chunks.
func (b *YaadBridge) ListIndexedPaths() ([]string, error) {
	if !b.ready {
		return nil, b.notReadyError("ListIndexedPaths")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.ListIndexedPaths(context.Background())
}

// SearchByType returns nodes matching the given type (label).
func (b *YaadBridge) SearchByType(nodeType string, limit int) ([]string, []string, error) {
	if !b.ready {
		return nil, nil, b.notReadyError("SearchByType")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nodes, err := b.store.ListNodes(context.Background(), storage.NodeFilter{Type: nodeType, Limit: limit})
	if err != nil {
		return nil, nil, err
	}
	ids := make([]string, len(nodes))
	contents := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
		contents[i] = n.Content
	}
	return ids, contents, nil
}

// UpdateNodeContent updates the content of a node by ID.
func (b *YaadBridge) UpdateNodeContent(id, newContent string) error {
	if !b.ready {
		return b.notReadyError("UpdateNodeContent")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.UpdateNodeContent(context.Background(), id, newContent)
}

// CompactResult is a lightweight search result (~50 tokens vs ~500 for full content).
type CompactResult struct {
	ID    string  `json:"id"`
	Type  string  `json:"type"`
	Title string  `json:"title"` // first 100 chars of content
	Score float64 `json:"score"`
}

// FullResult contains the complete content for a node.
type FullResult struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

// SearchCompact returns compact index entries (ID + title + score) without full content.
func (b *YaadBridge) SearchCompact(query string, limit int) ([]CompactResult, error) {
	if !b.ready {
		return nil, b.notReadyError("SearchCompact")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if limit <= 0 {
		limit = 10
	}
	nodes, err := b.store.SearchNodes(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}
	results := make([]CompactResult, len(nodes))
	for i, n := range nodes {
		title := n.Content
		if len(title) > 100 {
			title = title[:100]
		}
		results[i] = CompactResult{
			ID:    n.ID,
			Type:  n.Type,
			Title: title,
			Score: n.Confidence,
		}
	}
	return results, nil
}

// GetFullContent returns full content for specific node IDs.
func (b *YaadBridge) GetFullContent(ids []string) ([]FullResult, error) {
	if !b.ready {
		return nil, b.notReadyError("GetFullContent")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nodes, err := b.store.GetNodesBatch(context.Background(), ids)
	if err != nil {
		return nil, err
	}
	results := make([]FullResult, len(nodes))
	for i, n := range nodes {
		results[i] = FullResult{
			ID:      n.ID,
			Content: n.Content,
			Type:    n.Type,
		}
	}
	return results, nil
}

// Close shuts down the yaad engine and closes the database connection.
func (b *YaadBridge) Close() {
	if !b.ready {
		return
	}
	b.engine.Close()
	if b.store != nil {
		_ = b.store.Close()
	}
	b.ready = false
}

func bridgeDigest(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// LoadYaadContext queries yaad for project-relevant memories and returns
// a formatted string for injection into the system prompt.
// Uses a broad project-context search with a 2000-token budget.
// Returns empty string if yaad is unavailable or has no relevant memories.
func LoadYaadContext(projectDir string) string {
	bridge := NewYaadBridge()
	if !bridge.Ready() {
		return ""
	}

	// Search for project-specific context
	query := fmt.Sprintf("project context conventions decisions preferences for %s", filepath.Base(projectDir))
	content, err := bridge.Recall(query, 2000)
	if err != nil || content == "" {
		return ""
	}

	return "\n\n--- YAAD PROJECT MEMORY ---\n" + content + "\n--- END YAAD PROJECT MEMORY ---\n"
}

// YaadStatus returns a human-readable summary of yaad's state.
func YaadStatus() string {
	bridge := NewYaadBridge()
	if !bridge.Ready() {
		return "Yaad: not initialized (run 'yaad init' or ensure ~/.yaad/data/ exists)"
	}

	// Try to get a count of nodes
	_, _, err := bridge.SearchByType("convention", 1)
	if err != nil {
		return "Yaad: initialized but unable to query"
	}

	// Count different node types
	typeCounts := make(map[string]int)
	for _, nodeType := range []string{"convention", "decision", "bug", "preference", "task", "skill", "spec", "entity"} {
		ids, _, _ := bridge.SearchByType(nodeType, 1000)
		if len(ids) > 0 {
			typeCounts[nodeType] = len(ids)
		}
	}

	total := 0
	for _, count := range typeCounts {
		total += count
	}

	if total == 0 {
		return "Yaad: initialized but no memories stored"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Yaad: %d memories\n", total))
	for t, count := range typeCounts {
		b.WriteString(fmt.Sprintf("  %s: %d\n", t, count))
	}
	return b.String()
}

// FormatYaadDetail returns a human-readable dump of yaad memories for /yaad and diagnostics.
func FormatYaadDetail(maxPerType int) string {
	bridge := NewYaadBridge()
	if !bridge.Ready() {
		return "Yaad: not initialized\nEnsure ~/.yaad/data/ is writable. Hawk uses yaad as an embedded library (no separate daemon required)."
	}

	var b strings.Builder
	b.WriteString(YaadStatus())
	b.WriteString("\n\nRecent memories:\n")

	nodeTypes := []string{"convention", "decision", "bug", "preference", "task", "skill", "spec", "entity"}
	any := false
	for _, nodeType := range nodeTypes {
		_, contents, err := bridge.SearchByType(nodeType, maxPerType)
		if err != nil || len(contents) == 0 {
			continue
		}
		any = true
		b.WriteString(fmt.Sprintf("\n[%s]\n", nodeType))
		for i, content := range contents {
			line := strings.TrimSpace(content)
			if len(line) > 200 {
				line = line[:200] + "..."
			}
			b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, line))
		}
	}
	if !any {
		b.WriteString("  (none yet — hawk stores memories via CoreMemory tools and auto-remember)\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormatYaadSearch returns human-readable search results for yaad graph memory.
func FormatYaadSearch(query string, limit int) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "Yaad search: query required"
	}
	if limit < 1 {
		limit = 10
	}

	bridge := NewYaadBridge()
	if !bridge.Ready() {
		return "Yaad: not initialized\nEnsure ~/.yaad/data/ is writable."
	}

	results, err := bridge.SearchCompact(query, limit)
	if err != nil {
		return "Yaad search failed: " + err.Error()
	}
	if len(results) == 0 {
		return fmt.Sprintf("Yaad search: no memories matched %q", query)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Yaad search: %q (%d results)\n\n", query, len(results)))
	for i, r := range results {
		line := strings.TrimSpace(r.Title)
		if line == "" {
			line = "(empty)"
		}
		b.WriteString(fmt.Sprintf("%d. [%s] %s", i+1, r.Type, line))
		if r.Score > 0 {
			b.WriteString(fmt.Sprintf(" (%.2f)", r.Score))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
