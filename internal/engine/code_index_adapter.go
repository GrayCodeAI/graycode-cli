package engine

import (
	"github.com/GrayCodeAI/hawk/internal/intelligence/memory"
	"github.com/GrayCodeAI/hawk/internal/intelligence/repomap"
)

// yaadCodeIndexer adapts *memory.YaadBridge to repomap.CodeIndexer. The two
// packages define identical CodeSearchResult shapes under different names, so
// the adapter only has to convert the SearchCode return type; every other
// method forwards directly.
type yaadCodeIndexer struct {
	bridge *memory.YaadBridge
}

func (a *yaadCodeIndexer) IndexCodeChunk(path, content, symbol, lang string, start, end, tokens int, hash string) error {
	return a.bridge.IndexCodeChunk(path, content, symbol, lang, start, end, tokens, hash)
}

func (a *yaadCodeIndexer) SearchCode(query string, limit int) ([]repomap.CodeSearchResult, error) {
	results, err := a.bridge.SearchCode(query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]repomap.CodeSearchResult, 0, len(results))
	for _, r := range results {
		out = append(out, repomap.CodeSearchResult{
			Path: r.Path, StartLine: r.StartLine, EndLine: r.EndLine,
			Content: r.Content, Symbol: r.Symbol, Score: r.Score,
		})
	}
	return out, nil
}

func (a *yaadCodeIndexer) GetFileHash(path string) (string, error) {
	return a.bridge.GetFileHash(path)
}

func (a *yaadCodeIndexer) ClearFileChunks(path string) error {
	return a.bridge.ClearFileChunks(path)
}

func (a *yaadCodeIndexer) ListIndexedPaths() ([]string, error) {
	return a.bridge.ListIndexedPaths()
}
