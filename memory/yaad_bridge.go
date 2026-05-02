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
