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
