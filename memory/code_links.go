package memory

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/GrayCodeAI/yaad/storage"
)

// CodeMemoryLinker creates bidirectional links between indexed code chunks
// and memory graph nodes. When code is indexed, it auto-links to memories
// that reference that file. Enables "show all memories about this function."
type CodeMemoryLinker struct {
	bridge *YaadBridge
	mu     sync.Mutex
	cache  map[string][]string // file path → memory node IDs
}

// NewCodeMemoryLinker creates a linker that bridges code index and memory graph.
func NewCodeMemoryLinker(bridge *YaadBridge) *CodeMemoryLinker {
	return &CodeMemoryLinker{
		bridge: bridge,
		cache:  make(map[string][]string),
	}
}

// LinkFileToMemories finds all memory nodes that reference a file path
// and creates 'touches' edges between them and a file anchor node.
func (cl *CodeMemoryLinker) LinkFileToMemories(path string) error {
	if !cl.bridge.Ready() {
		return nil
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()

	ctx := context.Background()
	basename := filepath.Base(path)

	// Search for memories mentioning this file
	nodes, err := cl.bridge.store.SearchNodes(ctx, basename, 20)
	if err != nil || len(nodes) == 0 {
		return nil
	}

	// Find or create a file anchor node
	anchor := cl.getOrCreateFileAnchor(ctx, path)
	if anchor == nil {
		return nil
	}

	// Create 'touches' edges from memories to the file anchor
	linkedIDs := make([]string, 0)
	for _, node := range nodes {
		if node.ID == anchor.ID {
			continue
		}
		if !mentionsFile(node.Content, basename, path) {
			continue
		}
		edge := &storage.Edge{
			FromID: node.ID,
			ToID:   anchor.ID,
			Type:   "touches",
			Weight: 0.8,
		}
		if err := cl.bridge.store.CreateEdge(ctx, edge); err != nil {
			continue
		}
		linkedIDs = append(linkedIDs, node.ID)
	}

	cl.cache[path] = linkedIDs
	return nil
}

// MemoriesForFile returns all memory nodes linked to a specific file.
func (cl *CodeMemoryLinker) MemoriesForFile(path string) ([]string, error) {
	if !cl.bridge.Ready() {
		return nil, nil
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// Check cache first
	if ids, ok := cl.cache[path]; ok && len(ids) > 0 {
		return ids, nil
	}

	// Search for the file anchor
	basename := filepath.Base(path)
	nodes, err := cl.bridge.store.SearchNodes(context.Background(), basename, 20)
	if err != nil {
		return nil, err
	}

	var memoryIDs []string
	for _, n := range nodes {
		if mentionsFile(n.Content, basename, path) {
			memoryIDs = append(memoryIDs, n.ID)
		}
	}

	cl.cache[path] = memoryIDs
	return memoryIDs, nil
}

// MemoriesForSymbol returns memories related to a code symbol (function, class, etc).
func (cl *CodeMemoryLinker) MemoriesForSymbol(symbol string) ([]string, error) {
	if !cl.bridge.Ready() || symbol == "" {
		return nil, nil
	}

	nodes, err := cl.bridge.store.SearchNodes(context.Background(), symbol, 10)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, n := range nodes {
		if strings.Contains(strings.ToLower(n.Content), strings.ToLower(symbol)) {
			ids = append(ids, n.ID)
		}
	}
	return ids, nil
}

// OnFileChanged is called when a file is modified. It checks what memories
// are linked and returns impact information.
func (cl *CodeMemoryLinker) OnFileChanged(path string) []string {
	if !cl.bridge.Ready() {
		return nil
	}
	cl.mu.Lock()
	ids, ok := cl.cache[path]
	cl.mu.Unlock()

	if !ok {
		// Try to find links dynamically
		memIDs, _ := cl.MemoriesForFile(path)
		return memIDs
	}
	return ids
}

// InvalidateCache removes cached links for a file (call after file is deleted/moved).
func (cl *CodeMemoryLinker) InvalidateCache(path string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	delete(cl.cache, path)
}

func (cl *CodeMemoryLinker) getOrCreateFileAnchor(ctx context.Context, path string) *storage.Node {
	basename := filepath.Base(path)
	key := "file:" + path

	// Try to find existing anchor
	if node, err := cl.bridge.store.GetNodeByKey(ctx, key, ""); err == nil && node != nil {
		return node
	}

	// Create new file anchor
	node := &storage.Node{
		Type:       "file",
		Content:    "File: " + basename + " (" + path + ")",
		Scope:      "project",
		Tier:       2,
		Confidence: 0.9,
		Key:        key,
	}
	if err := cl.bridge.store.CreateNode(ctx, node); err != nil {
		return nil
	}
	return node
}

func mentionsFile(content, basename, fullPath string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, strings.ToLower(basename)) ||
		strings.Contains(lower, strings.ToLower(fullPath))
}
