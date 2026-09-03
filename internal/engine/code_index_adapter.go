package engine

import (
	"github.com/GrayCodeAI/graycode-cli/internal/intelligence/memory"
	"github.com/GrayCodeAI/graycode-cli/internal/intelligence/repomap"
)

// harrierCodeIndexer adapts *memory.HarrierBridge to repomap.CodeIndexer. The two
// packages define identical CodeSearchResult shapes under different names, so
// the adapter only has to convert the SearchCode return type; every other
// method forwards directly.
type harrierCodeIndexer struct {
	bridge *memory.HarrierBridge
}

func (a *harrierCodeIndexer) IndexCodeChunk(path, content, symbol, lang string, start, end, tokens int, hash string) error {
	return a.bridge.IndexCodeChunk(path, content, symbol, lang, start, end, tokens, hash)
}

func (a *harrierCodeIndexer) SearchCode(query string, limit int) ([]repomap.CodeSearchResult, error) {
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

func (a *harrierCodeIndexer) GetFileHash(path string) (string, error) {
	return a.bridge.GetFileHash(path)
}

func (a *harrierCodeIndexer) ClearFileChunks(path string) error {
	return a.bridge.ClearFileChunks(path)
}

func (a *harrierCodeIndexer) ListIndexedPaths() ([]string, error) {
	return a.bridge.ListIndexedPaths()
}
