package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	graphcontracts "github.com/GrayCodeAI/eagle/graph"
	"github.com/GrayCodeAI/graycode-cli/internal/graphjournal"
	"github.com/GrayCodeAI/graycode-cli/internal/graycodeerr"
	harrier "github.com/GrayCodeAI/harrier"
	harrierEngine "github.com/GrayCodeAI/harrier/engine"
	harrierGraph "github.com/GrayCodeAI/harrier/graph"
	"github.com/GrayCodeAI/harrier/portablegraph"
	harrierPortableGraph "github.com/GrayCodeAI/harrier/portablegraph/graph"
	"github.com/GrayCodeAI/harrier/storage"
)

// Backup tuning for the harrier snapshot scheduler. Snapshots go to
// ~/.harrier/data/backups/, kept hourly with a bounded window so an idle or
// crash-prone host always has a recent consistent copy of the memory DB.
const (
	harrierBackupDir      = "backups"
	harrierBackupInterval = time.Hour
	harrierBackupKeep     = 7
	harrierBackupMaxAge   = 30 * 24 * time.Hour
)

// harrierBackupsMu guards harrierBackupDirs. harrierBackupDirs tracks which backup
// directories already have a running scheduler so the many bridges a host
// creates (one per callsite) share a single loop per directory instead of
// stacking duplicate schedulers. Keying on the directory (not a global
// sync.Once) keeps tests with distinct temp homes isolated and still dedups
// the repeated NewHarrierBridge calls a real host makes against one home.
var (
	harrierBackupsMu  sync.Mutex
	harrierBackupDirs = make(map[string]struct{})
)

// HarrierBridge connects graycode's memory system to the harrier memory graph.
// If harrier is not initialized (missing DB), operations return a BridgeError
// and log a warning on first access.
type HarrierBridge struct {
	engine   *harrierEngine.Engine
	store    *storage.Store
	mu       sync.Mutex
	ready    bool
	warnOnce sync.Once

	dbDir       string
	backupSched *storage.BackupScheduler

	graphSessionID string
	graphScope     graphcontracts.Scope
}

// NewHarrierBridge initializes a bridge to harrier's SQLite store at ~/.harrier/data/harrier.db.
// Returns a bridge that silently no-ops if initialization fails.
func NewHarrierBridge() *HarrierBridge {
	b := &HarrierBridge{}
	b.init()
	return b
}

// harrierEncryptionKeyEnv names the environment variable supplying the
// at-rest encryption key for the harrier memory database. It is opt-in:
// without it, node content is stored in plaintext (the historical
// behaviour). Set it consistently on every process that opens the same
// database — content sealed without the key becomes unreadable.
const harrierEncryptionKeyEnv = "HARRIER_ENCRYPTION_KEY"

func (b *HarrierBridge) init() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dbDir := filepath.Join(home, ".harrier", "data")
	if mkErr := os.MkdirAll(dbDir, 0o700); mkErr != nil {
		return
	}
	dbPath := filepath.Join(dbDir, "harrier.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		return
	}

	// Opt-in at-rest encryption (PR harrier#59): when the key variable is set,
	// node content is sealed with AES-256-GCM before writing. An invalid key
	// disables the bridge entirely rather than silently storing plaintext
	// the user did not ask for.
	if v := strings.TrimSpace(os.Getenv(harrierEncryptionKeyEnv)); v != "" {
		if err := store.EnableEncryption(storage.NewEnvKeyProvider(harrierEncryptionKeyEnv)); err != nil {
			_ = store.Close()
			slog.Warn("[graycode/memory] harrier encryption key invalid; memory disabled", "error", err)
			return
		}
	}

	g := harrierGraph.New(store, store.DB())
	eng := harrierEngine.New(store, g)

	b.store = store
	b.engine = eng
	b.dbDir = dbDir
	b.ready = true
}

// EnsureBackups starts the harrier snapshot scheduler for the bridge's database
// directory. It is called once by the long-lived memory manager at session
// startup — not from NewHarrierBridge — so short-lived bridges (diagnostics,
// status queries, tests) never leave scheduler goroutines writing into
// their working directories. The directory is claimed so concurrent or
// repeated calls reuse one loop; Close releases the claim and stops the
// loop this bridge started.
func (b *HarrierBridge) EnsureBackups() {
	if b == nil || !b.ready {
		return
	}

	harrierBackupsMu.Lock()
	if _, busy := harrierBackupDirs[b.dbDir]; busy {
		harrierBackupsMu.Unlock()
		return
	}
	// Claim the directory under the lock so concurrent callers cannot both
	// start a scheduler; released on failure so a later caller can retry.
	harrierBackupDirs[b.dbDir] = struct{}{}
	harrierBackupsMu.Unlock()

	sched, err := b.store.ScheduleBackups(
		filepath.Join(b.dbDir, harrierBackupDir),
		harrierBackupInterval,
		harrierBackupKeep,
		harrierBackupMaxAge,
	)
	if err != nil {
		harrierBackupsMu.Lock()
		delete(harrierBackupDirs, b.dbDir)
		harrierBackupsMu.Unlock()
		slog.Warn("[graycode/memory] harrier backup scheduler not started", "error", err)
		return
	}
	sched.Start()

	b.mu.Lock()
	b.backupSched = sched
	b.mu.Unlock()
}

// Ready reports whether the harrier bridge is initialized and usable.
func (b *HarrierBridge) Ready() bool {
	return b.ready
}

// IsReady is a public alias for Ready, exported for external consumers
// that need to check bridge status before batching operations.
func (b *HarrierBridge) IsReady() bool {
	return b.ready
}

// ConfigureGraphObservation binds future successful recalls to a persisted
// Graycode session. Empty session IDs disable capture.
func (b *HarrierBridge) ConfigureGraphObservation(sessionID string, scope graphcontracts.Scope) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.graphSessionID = strings.TrimSpace(sessionID)
	b.graphScope = scope
	b.mu.Unlock()
}

// notReadyError logs a warning once and returns a structured BridgeError.
func (b *HarrierBridge) notReadyError(op string) error {
	b.warnOnce.Do(func() {
		slog.Warn("[graycode/memory] harrier bridge is not initialized; memory operations will be skipped", "hint", "ensure ~/.harrier/data/ is accessible")
	})
	return &graycodeerr.BridgeError{
		Bridge: "harrier",
		Op:     op,
		Reason: "bridge not initialized",
	}
}

// Remember stores content into harrier's memory graph under the given category.
// Category maps to harrier's node type (e.g., "convention", "decision", "bug", "preference").
// Returns a BridgeError if harrier is not initialized.
func (b *HarrierBridge) Remember(content, category string) error {
	return b.RememberWithContext(context.Background(), content, category)
}

// RememberWithContext is the context-aware version of Remember.
func (b *HarrierBridge) RememberWithContext(ctx context.Context, content, category string) error {
	if !b.ready {
		return b.notReadyError("Remember")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nodeType := category
	if !harrierEngine.IsValidNodeType(nodeType) {
		nodeType = "convention"
	}

	_, err := b.engine.Remember(ctx, harrierEngine.RememberInput{
		Type:    nodeType,
		Content: content,
		Scope:   "project",
	})
	return err
}

// Recall searches harrier's memory graph and returns formatted context that fits
// within the specified token budget. Returns a BridgeError if harrier is not initialized.
func (b *HarrierBridge) Recall(query string, tokenBudget int) (string, error) {
	return b.RecallWithContext(context.Background(), query, tokenBudget)
}

// RecallWithContext is the context-aware version of Recall.
func (b *HarrierBridge) RecallWithContext(ctx context.Context, query string, tokenBudget int) (string, error) {
	if !b.ready {
		return "", b.notReadyError("Recall")
	}

	result, err := b.recallResultWithContext(ctx, harrierEngine.RecallOpts{
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

func (b *HarrierBridge) recallResultWithContext(
	ctx context.Context,
	opts harrierEngine.RecallOpts,
) (*harrierEngine.RecallResult, error) {
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

// listNodes is the memory package's read boundary for Harrier node queries.
// Callers stay independent of the storage lifecycle and synchronization.
func (b *HarrierBridge) listNodes(ctx context.Context, filter storage.NodeFilter) ([]*storage.Node, error) {
	if !b.ready {
		return nil, b.notReadyError("ListNodes")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.store.ListNodes(ctx, filter)
}

func (b *HarrierBridge) listPinnedNodes(ctx context.Context, limit int) ([]*storage.Node, error) {
	pinned := true
	return b.listNodes(ctx, storage.NodeFilter{Pinned: &pinned, Limit: limit})
}

func (b *HarrierBridge) listNodesByType(ctx context.Context, nodeType string, minConfidence float64, limit int) ([]*storage.Node, error) {
	return b.listNodes(ctx, storage.NodeFilter{
		Type:          nodeType,
		MinConfidence: minConfidence,
		Limit:         limit,
	})
}

func (b *HarrierBridge) listNodesByScope(ctx context.Context, nodeType, scope string, minConfidence float64, limit int) ([]*storage.Node, error) {
	return b.listNodes(ctx, storage.NodeFilter{
		Type:          nodeType,
		Scope:         scope,
		MinConfidence: minConfidence,
		Limit:         limit,
	})
}

func (b *HarrierBridge) adjustNodeConfidence(ctx context.Context, id string, delta float64, skipPinned bool) error {
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

func (b *HarrierBridge) recallBudget(ctx context.Context, query string, budget, limit, depth int) (*harrierEngine.RecallResult, error) {
	return b.recallResultWithContext(ctx, harrierEngine.RecallOpts{
		Query:  query,
		Budget: budget,
		Limit:  limit,
		Depth:  depth,
	})
}

func (b *HarrierBridge) recallProject(ctx context.Context, query, project string, budget, limit, depth int) (*harrierEngine.RecallResult, error) {
	return b.recallResultWithContext(ctx, harrierEngine.RecallOpts{
		Query:   query,
		Budget:  budget,
		Limit:   limit,
		Depth:   depth,
		Project: project,
	})
}

func (b *HarrierBridge) rememberProject(ctx context.Context, content, nodeType, project, agent string) error {
	if !b.ready {
		return b.notReadyError("RememberProject")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if !harrierEngine.IsValidNodeType(nodeType) {
		nodeType = "convention"
	}
	_, err := b.engine.Remember(ctx, harrierEngine.RememberInput{
		Type:    nodeType,
		Content: content,
		Scope:   "project",
		Project: project,
		Agent:   agent,
	})
	return err
}

func (b *HarrierBridge) rememberGlobal(ctx context.Context, content, nodeType string) error {
	if !b.ready {
		return b.notReadyError("RememberGlobal")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	if !harrierEngine.IsValidNodeType(nodeType) {
		nodeType = "preference"
	}
	_, err := b.engine.Remember(ctx, harrierEngine.RememberInput{
		Type:    nodeType,
		Content: content,
		Scope:   "global",
		Project: "__global__",
	})
	return err
}

func (b *HarrierBridge) searchNodes(ctx context.Context, query string, limit int) ([]*storage.Node, error) {
	if !b.ready {
		return nil, b.notReadyError("SearchNodes")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.store.SearchNodes(ctx, query, limit)
}

func (b *HarrierBridge) createTouchEdge(ctx context.Context, fromID, toID string) error {
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

func (b *HarrierBridge) getOrCreateFileAnchor(ctx context.Context, path string) (*storage.Node, error) {
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

func (b *HarrierBridge) recordContextGraph(query string, result *harrierEngine.RecallResult) {
	if b.graphSessionID == "" || result == nil || len(result.Nodes) == 0 {
		return
	}
	projection, err := portablegraph.Build(portablegraph.Input{
		Nodes:           result.Nodes,
		Edges:           result.Edges,
		Query:           query,
		GeneratedAt:     time.Now(),
		Scope:           toHarrierScope(b.graphScope),
		ProducerVersion: harrier.Version,
	})
	if err != nil {
		slog.Warn("[graycode/memory] harrier context graph projection failed", "error", err)
		return
	}
	if err := graphjournal.AppendContextGraph(
		b.graphSessionID,
		"harrier",
		projection.QuerySHA256,
		toEagleNodes(projection.Nodes),
		toEagleEdges(projection.Edges),
		toEagleEvents(projection.Events),
		projection.GeneratedAt,
	); err != nil {
		slog.Warn("[graycode/memory] harrier context graph observation failed", "error", err)
	}
}

// The following helpers convert Harrier's vendored portable-graph contract
// types into Graycode's eagle/graph contract types (and the reverse for scope).
// The definitions are byte-identical, so conversion is a field-by-field copy
// at the sibling boundary.

func toHarrierScope(s graphcontracts.Scope) harrierPortableGraph.Scope {
	return harrierPortableGraph.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toEagleNodes(nodes []harrierPortableGraph.Node) []graphcontracts.Node {
	out := make([]graphcontracts.Node, len(nodes))
	for i, n := range nodes {
		out[i] = toEagleNode(n)
	}
	return out
}

func toEagleNode(n harrierPortableGraph.Node) graphcontracts.Node {
	return graphcontracts.Node{
		ID:          n.ID,
		Kind:        graphcontracts.NodeKind(n.Kind),
		Scope:       toEagleScope(n.Scope),
		CreatedAt:   n.CreatedAt,
		EffectiveAt: n.EffectiveAt,
		Provenance:  toEagleProvenance(n.Provenance),
		Attributes:  n.Attributes,
	}
}

func toEagleEdges(edges []harrierPortableGraph.Edge) []graphcontracts.Edge {
	out := make([]graphcontracts.Edge, len(edges))
	for i, e := range edges {
		out[i] = toEagleEdge(e)
	}
	return out
}

func toEagleEdge(e harrierPortableGraph.Edge) graphcontracts.Edge {
	return graphcontracts.Edge{
		ID:          e.ID,
		Kind:        graphcontracts.EdgeKind(e.Kind),
		From:        toEagleRef(e.From),
		To:          toEagleRef(e.To),
		Scope:       toEagleScope(e.Scope),
		CreatedAt:   e.CreatedAt,
		EffectiveAt: e.EffectiveAt,
		Provenance:  toEagleProvenance(e.Provenance),
		Attributes:  e.Attributes,
	}
}

func toEagleEvents(events []harrierPortableGraph.Event) []graphcontracts.Event {
	out := make([]graphcontracts.Event, len(events))
	for i, ev := range events {
		out[i] = toEagleEvent(ev)
	}
	return out
}

func toEagleEvent(ev harrierPortableGraph.Event) graphcontracts.Event {
	return graphcontracts.Event{
		ID:             ev.ID,
		Type:           graphcontracts.EventType(ev.Type),
		Subject:        toEagleRef(ev.Subject),
		Scope:          toEagleScope(ev.Scope),
		OccurredAt:     ev.OccurredAt,
		CorrelationID:  ev.CorrelationID,
		CausationID:    ev.CausationID,
		IdempotencyKey: ev.IdempotencyKey,
		Provenance:     toEagleProvenance(ev.Provenance),
	}
}

func toEagleRef(r harrierPortableGraph.Ref) graphcontracts.Ref {
	return graphcontracts.Ref{Kind: graphcontracts.NodeKind(r.Kind), ID: r.ID}
}

func toEagleScope(s harrierPortableGraph.Scope) graphcontracts.Scope {
	return graphcontracts.Scope{TenantID: s.TenantID, ProjectID: s.ProjectID, RepositoryID: s.RepositoryID}
}

func toEagleProvenance(p harrierPortableGraph.Provenance) graphcontracts.Provenance {
	evidence := make([]graphcontracts.ArtifactRef, len(p.Evidence))
	for i, a := range p.Evidence {
		evidence[i] = graphcontracts.ArtifactRef{URI: a.URI, Digest: a.Digest, MediaType: a.MediaType}
	}
	return graphcontracts.Provenance{Producer: p.Producer, Version: p.Version, SourceID: p.SourceID, Evidence: evidence}
}

func (b *HarrierBridge) recordSelectedContext(label string, nodes []*storage.Node) {
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
		slog.Warn("[graycode/memory] harrier selected context edge lookup failed", "error", err)
		edges = nil
	}
	b.recordContextGraph(label, &harrierEngine.RecallResult{Nodes: nodes, Edges: edges})
}

// InitCodeIndex creates the code index tables in harrier's store.
// Safe to call multiple times. Returns a BridgeError if harrier is not initialized.
func (b *HarrierBridge) InitCodeIndex() error {
	if !b.ready {
		return b.notReadyError("InitCodeIndex")
	}
	return b.store.CreateCodeIndex(context.Background())
}

// IndexCodeChunk stores a code chunk in the harrier code index.
func (b *HarrierBridge) IndexCodeChunk(path, content, symbol, lang string, start, end, tokens int, hash string) error {
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
func (b *HarrierBridge) SearchCode(query string, limit int) ([]CodeSearchResult, error) {
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

func (b *HarrierBridge) recordCodeContext(query string, records []*storage.CodeChunkRecord) {
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
			ID:   "graycode/code-chunk/" + sourceID,
		}
		node := graphcontracts.Node{
			ID:        ref.ID,
			Kind:      ref.Kind,
			Scope:     b.graphScope,
			CreatedAt: occurredAt,
			Provenance: graphcontracts.Provenance{
				Producer: "graycode",
				SourceID: sourceID,
				Evidence: []graphcontracts.ArtifactRef{{URI: "graycode://code-index/" + sourceID}},
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
			ID:             "graycode/event/code-chunk/" + sourceID + "/observed/" + querySHA256,
			Type:           graphcontracts.EventObserved,
			Subject:        ref,
			Scope:          b.graphScope,
			OccurredAt:     occurredAt,
			CorrelationID:  querySHA256,
			IdempotencyKey: "graycode/code-chunk/" + sourceID + "/observed/" + querySHA256,
			Provenance:     node.Provenance,
		})
	}
	if err := graphjournal.AppendContextGraph(
		b.graphSessionID,
		"graycode-index",
		querySHA256,
		nodes,
		nil,
		events,
		occurredAt,
	); err != nil {
		slog.Warn("[graycode/memory] code context graph observation failed", "error", err)
	}
}

// GetFileHash returns the stored hash for a file path, or empty string if not indexed.
func (b *HarrierBridge) GetFileHash(path string) (string, error) {
	if !b.ready {
		return "", b.notReadyError("GetFileHash")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.GetFileHash(context.Background(), path)
}

// ClearFileChunks removes all indexed chunks for a given file path.
func (b *HarrierBridge) ClearFileChunks(path string) error {
	if !b.ready {
		return b.notReadyError("ClearFileChunks")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.DeleteChunksByPath(context.Background(), path)
}

// ListIndexedPaths returns all file paths that have indexed code chunks.
func (b *HarrierBridge) ListIndexedPaths() ([]string, error) {
	if !b.ready {
		return nil, b.notReadyError("ListIndexedPaths")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.ListIndexedPaths(context.Background())
}

// SearchByType returns nodes matching the given type (label).
func (b *HarrierBridge) SearchByType(nodeType string, limit int) ([]string, []string, error) {
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
func (b *HarrierBridge) UpdateNodeContent(id, newContent string) error {
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
func (b *HarrierBridge) SearchCompact(query string, limit int) ([]CompactResult, error) {
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
func (b *HarrierBridge) GetFullContent(ids []string) ([]FullResult, error) {
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

// Close shuts down the harrier engine and closes the database connection.
// If this bridge started the backup scheduler, it is stopped here and the
// directory claim released so a later bridge can restart snapshots.
func (b *HarrierBridge) Close() {
	b.mu.Lock()
	sched := b.backupSched
	b.backupSched = nil
	wasReady := b.ready
	b.ready = false
	b.mu.Unlock()

	if sched != nil {
		sched.Stop()
		harrierBackupsMu.Lock()
		delete(harrierBackupDirs, b.dbDir)
		harrierBackupsMu.Unlock()
	}

	if !wasReady {
		return
	}
	b.engine.Close()
	if b.store != nil {
		_ = b.store.Close()
	}
}

func bridgeDigest(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// LoadHarrierContext queries harrier for project-relevant memories and returns
// a formatted string for injection into the system prompt.
// Uses a broad project-context search with a 2000-token budget.
// Returns empty string if harrier is unavailable or has no relevant memories.
func LoadHarrierContext(projectDir string) string {
	bridge := NewHarrierBridge()
	if !bridge.Ready() {
		return ""
	}

	// Search for project-specific context
	query := fmt.Sprintf("project context conventions decisions preferences for %s", filepath.Base(projectDir))
	content, err := bridge.Recall(query, 2000)
	if err != nil || content == "" {
		return ""
	}

	return "\n\n--- HARRIER PROJECT MEMORY ---\n" + content + "\n--- END HARRIER PROJECT MEMORY ---\n"
}

// HarrierStatus returns a human-readable summary of harrier's state.
func HarrierStatus() string {
	bridge := NewHarrierBridge()
	if !bridge.Ready() {
		return "Harrier: not initialized (run 'harrier init' or ensure ~/.harrier/data/ exists)"
	}

	// Try to get a count of nodes
	_, _, err := bridge.SearchByType("convention", 1)
	if err != nil {
		return "Harrier: initialized but unable to query"
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
		return "Harrier: initialized but no memories stored"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Harrier: %d memories\n", total))
	for t, count := range typeCounts {
		b.WriteString(fmt.Sprintf("  %s: %d\n", t, count))
	}
	return b.String()
}

// FormatHarrierDetail returns a human-readable dump of harrier memories for /harrier and diagnostics.
func FormatHarrierDetail(maxPerType int) string {
	bridge := NewHarrierBridge()
	if !bridge.Ready() {
		return "Harrier: not initialized\nEnsure ~/.harrier/data/ is writable. Graycode uses harrier as an embedded library (no separate daemon required)."
	}

	var b strings.Builder
	b.WriteString(HarrierStatus())
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
		b.WriteString("  (none yet — graycode stores memories via CoreMemory tools and auto-remember)\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormatHarrierSearch returns human-readable search results for harrier graph memory.
func FormatHarrierSearch(query string, limit int) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "Harrier search: query required"
	}
	if limit < 1 {
		limit = 10
	}

	bridge := NewHarrierBridge()
	if !bridge.Ready() {
		return "Harrier: not initialized\nEnsure ~/.harrier/data/ is writable."
	}

	results, err := bridge.SearchCompact(query, limit)
	if err != nil {
		return "Harrier search failed: " + err.Error()
	}
	if len(results) == 0 {
		return fmt.Sprintf("Harrier search: no memories matched %q", query)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Harrier search: %q (%d results)\n\n", query, len(results)))
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
