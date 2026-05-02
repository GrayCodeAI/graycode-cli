package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	yaadEngine "github.com/GrayCodeAI/yaad/engine"
	"github.com/GrayCodeAI/yaad/graph"
	"github.com/GrayCodeAI/yaad/storage"
)

// YaadBridge connects hawk's memory system to the yaad memory graph.
// If yaad is not initialized (missing DB), all operations fall back silently.
type YaadBridge struct {
	engine *yaadEngine.Engine
	store  *storage.Store
	mu     sync.Mutex
	ready  bool
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
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return
	}
	dbPath := filepath.Join(dbDir, "yaad.db")

	store, err := storage.NewStore(dbPath)
	if err != nil {
		return
	}

	g := graph.New(store, store.DB())
	eng := yaadEngine.New(store, g)

	b.store = store
	b.engine = eng
	b.ready = true
}

// Ready reports whether the yaad bridge is initialized and usable.
func (b *YaadBridge) Ready() bool {
	return b.ready
}

// Remember stores content into yaad's memory graph under the given category.
// Category maps to yaad's node type (e.g., "convention", "decision", "bug", "preference").
// Falls back silently if yaad is not initialized.
func (b *YaadBridge) Remember(content, category string) error {
	if !b.ready {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	nodeType := category
	if !yaadEngine.IsValidNodeType(nodeType) {
		nodeType = "convention"
	}

	_, err := b.engine.Remember(context.Background(), yaadEngine.RememberInput{
		Type:    nodeType,
		Content: content,
		Scope:   "project",
	})
	return err
}

// Recall searches yaad's memory graph and returns formatted context that fits
// within the specified token budget. Falls back silently returning empty string
// if yaad is not initialized.
func (b *YaadBridge) Recall(query string, tokenBudget int) (string, error) {
	if !b.ready {
		return "", nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	result, err := b.engine.Recall(context.Background(), yaadEngine.RecallOpts{
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

// InitCodeIndex creates the code index tables in yaad's store.
// Safe to call multiple times.
func (b *YaadBridge) InitCodeIndex() error {
	if !b.ready {
		return nil
	}
	return b.store.CreateCodeIndex(context.Background())
}

// IndexCodeChunk stores a code chunk in the yaad code index.
func (b *YaadBridge) IndexCodeChunk(path, content, symbol, lang string, start, end, tokens int, hash string) error {
	if !b.ready {
		return nil
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
		return nil, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	records, err := b.store.SearchCodeChunksFTS(context.Background(), query, limit)
	if err != nil {
		return nil, err
	}

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

// GetFileHash returns the stored hash for a file path, or empty string if not indexed.
func (b *YaadBridge) GetFileHash(path string) (string, error) {
	if !b.ready {
		return "", nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.GetFileHash(context.Background(), path)
}

// ClearFileChunks removes all indexed chunks for a given file path.
func (b *YaadBridge) ClearFileChunks(path string) error {
	if !b.ready {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.DeleteChunksByPath(context.Background(), path)
}

// ListIndexedPaths returns all file paths that have indexed code chunks.
func (b *YaadBridge) ListIndexedPaths() ([]string, error) {
	if !b.ready {
		return nil, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.store.ListIndexedPaths(context.Background())
}

// SearchByType returns nodes matching the given type (label).
func (b *YaadBridge) SearchByType(nodeType string, limit int) ([]string, []string, error) {
	if !b.ready {
		return nil, nil, nil
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
		return nil
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
		return nil, nil
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
	if !b.ready || len(ids) == 0 {
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
		b.store.Close()
	}
	b.ready = false
}
